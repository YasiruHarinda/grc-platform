// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { UserMenu } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useState, useEffect, useRef } from "react";
import { useAsgardeo } from "@asgardeo/react";
import { LogOut, UserRound } from "@wso2/oxygen-ui-icons-react";
import { useLogger } from "@hooks/useLogger";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import UserProfileModal from "./UserProfileModal";

interface MyProfile {
  first_name: string;
  last_name: string;
  thumbnail_url: string;
}

const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

function MockUserProfile(): JSX.Element {
  const [profileOpen, setProfileOpen] = useState(false);

  return (
    <>
      <UserMenu>
        <UserMenu.Trigger name="Dev User" />
        <UserMenu.Header name="Dev User" email="dev@localhost" />
        <UserMenu.Divider />
        <UserMenu.Item
          icon={<UserRound size={18} />}
          label="Profile"
          onClick={() => setProfileOpen(true)}
        />
        <UserMenu.Divider />
        <UserMenu.Logout
          icon={<LogOut size={18} />}
          label="Log out"
          onClick={() => {}}
        />
      </UserMenu>
      <UserProfileModal
        open={profileOpen}
        onClose={() => setProfileOpen(false)}
      />
    </>
  );
}

export default function UserProfile(): JSX.Element {
  const { signOut, isLoading, isSignedIn, getDecodedIdToken } = useAsgardeo();
  const authFetch = useAuthApiClient();
  const logger = useLogger();
  const [email, setEmail] = useState("");
  // Real name/picture from Asgardeo's own given_name/family_name/picture
  // claims. Historically empty because this org's application isn't
  // configured to release them; the claims may also arrive partially (a
  // given_name with no family_name), which is why the name below is chosen by
  // completeness rather than by whether this is merely non-empty. The picture
  // still takes priority whenever Asgardeo supplies one.
  const [asgardeoName, setAsgardeoName] = useState("");
  const [asgardeoPicture, setAsgardeoPicture] = useState<string | null>(null);
  // Decent guess derived from the email prefix (e.g. "asel.fernando" ->
  // "Asel Fernando"), used when neither Asgardeo nor hr_entity have a name.
  const [emailDerivedName, setEmailDerivedName] = useState("");
  // Last-resort fallback if nothing else has a name — the raw
  // username/subject identifier, not a real display name.
  const [desperateFallbackName, setDesperateFallbackName] = useState("");
  const [profile, setProfile] = useState<MyProfile | null>(null);
  const [profileOpen, setProfileOpen] = useState(false);
  // Both effects below key on isSignedIn (a primitive) only, and guard with
  // a ref, deliberately excluding getDecodedIdToken/authFetch/logger from
  // their dependency arrays — those come from the Asgardeo SDK hook and
  // aren't guaranteed to be reference-stable across renders, which was
  // retriggering these effects (and their network calls) continuously.
  // Same pattern digiops-hr apps use to fetch employee profile data once.
  const hasFetchedIdentityRef = useRef(false);
  const hasFetchedProfileRef = useRef(false);

  useEffect(() => {
    if (isMockAuth || !isSignedIn) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- clear stale identity when user signs out
      setEmail("");
      setAsgardeoName("");
      setAsgardeoPicture(null);
      setEmailDerivedName("");
      setDesperateFallbackName("");
      setProfile(null);
      hasFetchedIdentityRef.current = false;
      hasFetchedProfileRef.current = false;
      return;
    }
    if (hasFetchedIdentityRef.current) return;
    hasFetchedIdentityRef.current = true;
    getDecodedIdToken()
      .then((token) => {
        const given = token?.given_name ?? "";
        const family = token?.family_name ?? "";
        const email = token?.email ?? "";
        const emailPrefix = email.split("@")[0];
        const emailFallback = emailPrefix
          .split(/[._-]/)
          .filter(Boolean)
          .map((w: string) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
          .join(" ");
        setAsgardeoName([given, family].filter(Boolean).join(" "));
        setAsgardeoPicture((token as Record<string, string>)?.picture ?? null);
        setEmailDerivedName(emailFallback);
        setDesperateFallbackName((token as Record<string, string>)?.username || token?.sub || "");
        setEmail(email);
      })
      .catch((error) => {
        setAsgardeoName("");
        setAsgardeoPicture(null);
        setEmailDerivedName("");
        setDesperateFallbackName("");
        setEmail("");
        logger.error("Failed to decode ID token", error);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- see comment above hasFetchedIdentityRef
  }, [isSignedIn]);

  // Fallback name/photo from hr_entity (looked up server-side by the
  // signed-in user's own email) — used only when Asgardeo doesn't have
  // given_name/family_name/picture for this user (see asgardeoName's
  // comment above). hr_entity's own thumbnail is frequently empty too
  // (PeopleHR-sourced data, not actively maintained), so this is itself
  // a fallback, not the primary source.
  useEffect(() => {
    if (isMockAuth || !isSignedIn || hasFetchedProfileRef.current) return;
    hasFetchedProfileRef.current = true;
    authFetch(`${BACKEND_BASE_URL}/api/v1/me/profile`)
      .then((res) => (res.ok ? (res.json() as Promise<MyProfile>) : null))
      .then((data) => {
        if (data) setProfile(data);
      })
      .catch((error) => {
        logger.error("Failed to load HR entity profile", error);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- see comment above hasFetchedIdentityRef
  }, [isSignedIn]);

  if (isMockAuth) return <MockUserProfile />;

  const handleLogout = async () => {
    window.dispatchEvent(new CustomEvent("app:signing-out"));
    try {
      await signOut();
    } catch (error) {
      logger.error("Failed to sign out", error);
    }
  };

  if (isLoading || !isSignedIn) return <></>;

  // Name priority: hr_entity first, then Asgardeo's claims, then a guess from
  // the email prefix, then the raw username/subject so the menu is never blank.
  //
  // Both sources can be partially populated — Asgardeo may release given_name
  // without family_name, and hr_entity may hold a first name with no last name
  // — so neither is trusted just for being non-empty. Whichever supplies more
  // name parts wins, with hr_entity taking ties as the more reliable source of
  // a person's full name. A plain `asgardeoName || profileName` chain got this
  // wrong: a one-word Asgardeo name is truthy, so it shadowed a complete
  // hr_entity name and the account menu showed only a first name.
  const profileName = [profile?.first_name, profile?.last_name].filter(Boolean).join(" ");
  const fullerName = (a: string, b: string): string => {
    const parts = (s: string) => (s.trim() === "" ? 0 : s.trim().split(/\s+/).length);
    return parts(a) >= parts(b) ? a : b;
  };
  const resolvedName = fullerName(profileName, asgardeoName);
  const displayName = resolvedName || emailDerivedName || desperateFallbackName || "User";
  const displayEmail = email || " ";
  const picture = asgardeoPicture || profile?.thumbnail_url || null;

  return (
    <>
      <UserMenu>
        <UserMenu.Trigger name={displayName} avatar={picture} />
        <UserMenu.Header name={displayName} email={displayEmail} avatar={picture} />
        <UserMenu.Divider />
        <UserMenu.Item
          icon={<UserRound size={18} />}
          label="Profile"
          onClick={() => setProfileOpen(true)}
        />
        <UserMenu.Divider />
        <UserMenu.Logout
          icon={<LogOut size={18} />}
          label="Log out"
          onClick={handleLogout}
        />
      </UserMenu>
      <UserProfileModal
        open={profileOpen}
        onClose={() => setProfileOpen(false)}
      />
    </>
  );
}
