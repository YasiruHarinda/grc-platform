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
import { useState, useEffect } from "react";
import { useAsgardeo } from "@asgardeo/react";
import { LogOut } from "@wso2/oxygen-ui-icons-react";
import { useLogger } from "@hooks/useLogger";

const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

function MockUserProfile(): JSX.Element {
  return (
    <UserMenu>
      <UserMenu.Trigger name="Dev User" />
      <UserMenu.Header name="Dev User" email="dev@localhost" />
      <UserMenu.Divider />
      <UserMenu.Logout
        icon={<LogOut size={18} />}
        label="Log out"
        onClick={() => {}}
      />
    </UserMenu>
  );
}

export default function UserProfile(): JSX.Element {
  const { signOut, isLoading, isSignedIn, getDecodedIdToken } = useAsgardeo();
  const logger = useLogger();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");

  useEffect(() => {
    if (isMockAuth || !isSignedIn) return;
    getDecodedIdToken()
      .then((token) => {
        const given = token?.given_name ?? "";
        const family = token?.family_name ?? "";
        const resolved =
          [given, family].filter(Boolean).join(" ") ||
          (token as Record<string, string>)?.username ||
          token?.sub ||
          "";
        setName(resolved);
        setEmail(token?.email ?? "");
      })
      .catch(() => {});
  }, [isSignedIn, getDecodedIdToken]);

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

  const displayName = name || "User";
  const displayEmail = email || " ";

  return (
    <UserMenu>
      <UserMenu.Trigger name={displayName} />
      <UserMenu.Header name={displayName} email={displayEmail} />
      <UserMenu.Divider />
      <UserMenu.Logout
        icon={<LogOut size={18} />}
        label="Log out"
        onClick={handleLogout}
      />
    </UserMenu>
  );
}
