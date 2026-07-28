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

import { useEffect, useState } from "react";
import { FormProvider, useForm } from "react-hook-form";
import type { FieldPath } from "react-hook-form";
import { useAsgardeo } from "@asgardeo/react";
import {
  Alert,
  Box,
  Button,
  Divider,
  Paper,
  Stack,
  Step,
  StepLabel,
  Stepper,
  Typography,
} from "@wso2/oxygen-ui";
import { ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import BasicInformationStep from "./add-risk/BasicInformationStep";
import RiskAssessmentStep from "./add-risk/RiskAssessmentStep";
import ActionPlanStep from "./add-risk/ActionPlanStep";
import { buildRiskCode, getCurrentQuarter, getCurrentYear } from "./add-risk/constants";
import type { AddRiskFormValues } from "./add-risk/types";
import { darkCardSx } from "./cardStyles";
import {
  createRisk,
  fetchAssignmentTeams,
  fetchComplianceReferences,
  fetchNextSequenceID,
  fetchRiskScores,
  fetchSourceRegisterTeams,
  fetchUsers,
} from "../api/riskApi";
import type { ComplianceReference, RiskScore, RiskTeam, UserOption } from "../api/riskApi";
import { useAuthApiClient } from "@hooks/useAuthApiClient";

const STEPS = ["Basic Information", "Risk Assessment", "Risk Treatment Plan"] as const;

// Fields validated when the user clicks Next on each step.
const STEP_1_FIELDS: (keyof AddRiskFormValues)[] = [
  "year",
  "quarter",
  "sourceRegister",
  "riskTitle",
  "riskDescription",
  "identifiedByType",
  "assignedBy",
  "riskIdentifiedDate",
];

const STEP_2_FIELDS: (keyof AddRiskFormValues)[] = [
  "likelihood",
  "impact",
  "impactDescription",
  "implementationDate",
  "reassessmentDate",
];

function SuccessState({ onReset }: { onReset: () => void }): JSX.Element {
  return (
    <Stack alignItems="center" justifyContent="center" gap={2} sx={{ py: 8, textAlign: "center" }}>
      <Box sx={{ color: "success.main" }}>
        <ShieldCheck size={48} />
      </Box>
      <Typography variant="h5" fontWeight={600}>
        Risk Submitted Successfully
      </Typography>
      <Typography variant="body2" color="text.secondary">
        The risk has been registered and is now pending compliance review.
        Your risk code will be confirmed in the Risk Registers.
      </Typography>
      <Stack direction="row" gap={2} sx={{ mt: 2 }}>
        <Button variant="outlined" onClick={onReset}>
          Add Another Risk
        </Button>
        <Button variant="contained" href="/risk/registers">
          View Risk Registers
        </Button>
      </Stack>
    </Stack>
  );
}

interface RiskCodeConflict {
  taken: string;
  next: string;
}

export default function AddRisk(): JSX.Element {
  const [activeStep, setActiveStep] = useState(0);
  const [riskSequenceId, setRiskSequenceId] = useState<number | null>(null);
  const [riskCodeConflict, setRiskCodeConflict] = useState<RiskCodeConflict | null>(null);

  // Fetched lookup data for dropdowns
  const [sourceRegisterTeams, setSourceRegisterTeams] = useState<RiskTeam[]>([]);
  const [assignmentTeams, setAssignmentTeams]         = useState<RiskTeam[]>([]);
  const [riskScores, setRiskScores]                   = useState<RiskScore[]>([]);
  const [complianceRefs, setComplianceRefs]           = useState<ComplianceReference[]>([]);
  const [users, setUsers]                             = useState<UserOption[]>([]);
  const [fetchError, setFetchError]                   = useState<string | null>(null);
  const [submitError, setSubmitError]                 = useState<string | null>(null);

  const { getDecodedIdToken, isSignedIn } = useAsgardeo();
  const authFetch = useAuthApiClient();

  const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

  const methods = useForm<AddRiskFormValues>({
    defaultValues: {
      year: getCurrentYear(),
      quarter: getCurrentQuarter(),
      sourceRegister: "",
      riskTitle: "",
      riskDescription: "",
      complianceReferences: [],
      identifiedByType: "EMPLOYEE",
      identifiedByName: "",
      identifiedByEmail: "",
      assignedBy: "",
      riskIdentifiedDate: null,
      // ── Step 2 defaults ───────────────────────────────────────────────────
      likelihood: null,
      impact: null,
      impactDescription: "",
      implementationDate: null,
      reassessmentDate: new Date(),
      // ── Step 3 defaults ───────────────────────────────────────────────────
      assignmentTeam: "",
      riskOwner: "",
      actionOwner: "",
      actionPlanDescription: "",
      actionSteps: [{ description: "" }],
      treatmentStrategy: "",
      progress: "",
      gitIssueUrl: "",
      emailSubject: "",
      remarks: "",
      evidenceAttachments: [],
    },
    mode: "onSubmit",
  });

  const { trigger, handleSubmit, setError } = methods;

  // Watch the three fields that determine the risk code preview and next-sequence-id.
  const watchedYear            = methods.watch("year");
  const watchedQuarter         = methods.watch("quarter");
  const watchedSourceRegister  = methods.watch("sourceRegister");

  useEffect(() => {
    document.getElementById("main-scroll-container")?.scrollTo({ top: 0 });
  }, [activeStep]);

  // Fetch all static dropdown data once the user is ready (real auth or mock mode).
  useEffect(() => {
    if (!isSignedIn && !isMockAuth) return;
    setFetchError(null);
    Promise.all([
      fetchSourceRegisterTeams(authFetch),
      fetchAssignmentTeams(authFetch),
      fetchRiskScores(authFetch),
      fetchComplianceReferences(authFetch),
      fetchUsers(authFetch),
    ])
      .then(([srTeams, atTeams, scores, refs, userList]) => {
        setSourceRegisterTeams(srTeams);
        setAssignmentTeams(atTeams);
        setRiskScores(scores);
        setComplianceRefs(refs);
        setUsers(userList);
      })
      .catch(() => {
        setFetchError("Failed to load form data. Please refresh the page.");
      });
  }, [isSignedIn, isMockAuth, authFetch]);

  // Pre-fill assignedBy with the current signed-in user once the user list is loaded.
  // Skipped in mock mode — no real decoded token is available.
  useEffect(() => {
    if (isMockAuth || !isSignedIn || users.length === 0) return;
    getDecodedIdToken()
      .then((token) => {
        const email = token?.email as string | undefined;
        if (!email) return;
        const me = users.find((u) => u.email === email);
        if (me) methods.setValue("assignedBy", me.id, { shouldDirty: false });
      })
      .catch(() => {});
  }, [isSignedIn, isMockAuth, users, getDecodedIdToken]);

  // Re-fetch the next sequence ID whenever year, quarter, or source register changes.
  useEffect(() => {
    if (typeof watchedSourceRegister !== "number") {
      setRiskSequenceId(null);
      return;
    }
    if (!isSignedIn && !isMockAuth) return;
    fetchNextSequenceID(authFetch, watchedSourceRegister, watchedYear, watchedQuarter)
      .then(setRiskSequenceId)
      .catch(() => setRiskSequenceId(null));
  }, [watchedYear, watchedQuarter, watchedSourceRegister, isSignedIn, isMockAuth, authFetch]);

  const isLastStep = activeStep === STEPS.length - 1;
  const isComplete = activeStep === STEPS.length;

  const handleNext = async (): Promise<void> => {
    let valid = true;

    if (activeStep === 0) {
      valid = await trigger([...STEP_1_FIELDS, "identifiedByName"]);
    } else if (activeStep === 1) {
      valid = await trigger(STEP_2_FIELDS);
    }

    if (valid) setActiveStep((prev) => prev + 1);
  };

  const handleBack = (): void => setActiveStep((prev) => prev - 1);

  const handleReset = (): void => {
    setActiveStep(0);
    setRiskCodeConflict(null);
    setRiskSequenceId(null);
    setSubmitError(null);
    methods.reset();
  };

  const onSubmit = async (data: AddRiskFormValues): Promise<void> => {
    // Validate step 3 required fields manually — no RHF rules on these to avoid premature errors.
    let hasStep3Error = false;
    if (!data.assignmentTeam) {
      setError("assignmentTeam", { type: "required", message: "Assignment team is required" });
      hasStep3Error = true;
    }
    if (!data.riskOwner) {
      setError("riskOwner", { type: "required", message: "Risk owner is required" });
      hasStep3Error = true;
    }
    if (!data.actionOwner) {
      setError("actionOwner", { type: "required", message: "Action owner is required" });
      hasStep3Error = true;
    }
    if (!data.treatmentStrategy) {
      setError("treatmentStrategy", { type: "required", message: "Treatment strategy is required" });
      hasStep3Error = true;
    }
    if (!data.emailSubject?.trim()) {
      setError("emailSubject", { type: "required", message: "Email subject is required" });
      hasStep3Error = true;
    }
    data.actionSteps.forEach((step, i) => {
      if (!step.description?.trim()) {
        setError(`actionSteps.${i}.description` as FieldPath<AddRiskFormValues>, { type: "required", message: "Step description is required" });
        hasStep3Error = true;
      }
    });
    if (hasStep3Error) return;

    setSubmitError(null);
    try {
      await createRisk(authFetch, data);
      setActiveStep(STEPS.length);
    } catch (err: unknown) {
      const apiErr = err as { status?: number; message?: string; data?: { next_sequence_id?: number } };
      if (apiErr.status === 409 && typeof data.sourceRegister === "number" && riskSequenceId !== null) {
        const nextSeqId = apiErr.data?.next_sequence_id ?? riskSequenceId + 1;
        const teamCode = sourceRegisterTeams.find(t => t.id === data.sourceRegister)?.code
          ?? String(data.sourceRegister);
        setRiskCodeConflict({
          taken: buildRiskCode(data.year, teamCode, data.quarter, riskSequenceId),
          next:  buildRiskCode(data.year, teamCode, data.quarter, nextSeqId),
        });
        setRiskSequenceId(nextSeqId);
      } else {
        setSubmitError(apiErr.message ?? "Failed to submit risk. Please try again.");
      }
    }
  };

  const stepContent: JSX.Element[] = [
    <BasicInformationStep
      riskSequenceId={riskSequenceId}
      sourceRegisterTeams={sourceRegisterTeams}
      complianceRefs={complianceRefs}
      users={users}
    />,
    <RiskAssessmentStep riskScores={riskScores} />,
    <ActionPlanStep assignmentTeams={assignmentTeams} users={users} />,
  ];

  return (
    <Box sx={{ p: { xs: 2, sm: 3 }, maxWidth: 1500, mx: "auto" }}>
      <Typography variant="h4" fontWeight={700} sx={{ mb: 0.5 }}>
        Add a Risk
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Complete all three steps to register a new risk in the system.
      </Typography>

      <FormProvider {...methods}>
        <Paper
          component="form"
          variant="outlined"
          sx={{
            p: { xs: 2, sm: 4 },
            borderRadius: 2,
            ...darkCardSx,
          }}
          onSubmit={(e) => e.preventDefault()}
          noValidate
        >
          <Stepper activeStep={activeStep} sx={{ mb: 4 }}>
            {STEPS.map((label) => (
              <Step key={label}>
                <StepLabel>{label}</StepLabel>
              </Step>
            ))}
          </Stepper>

          <Divider sx={{ mb: 4 }} />

          {isComplete ? (
            <SuccessState onReset={handleReset} />
          ) : (
            <Stack gap={4}>
              {fetchError && (
                <Alert severity="error">
                  {fetchError}
                </Alert>
              )}

              {riskCodeConflict && (
                <Alert
                  severity="warning"
                  onClose={() => setRiskCodeConflict(null)}
                >
                  Risk code <strong>{riskCodeConflict.taken}</strong> was just claimed by another
                  submission. Your new risk code is <strong>{riskCodeConflict.next}</strong>.
                  Please review and resubmit.
                </Alert>
              )}

              {submitError && (
                <Alert severity="error" onClose={() => setSubmitError(null)}>
                  {submitError}
                </Alert>
              )}

              {stepContent[activeStep]}

              <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Button
                  type="button"
                  variant="outlined"
                  onClick={handleBack}
                  disabled={activeStep === 0}
                >
                  Back
                </Button>

                <Typography variant="body2" color="text.secondary">
                  Step {activeStep + 1} of {STEPS.length}
                </Typography>

                {isLastStep ? (
                  <Button variant="contained" type="button" onClick={() => handleSubmit(onSubmit)()}>
                    Submit
                  </Button>
                ) : (
                  <Button type="button" variant="contained" onClick={handleNext}>
                    Next
                  </Button>
                )}
              </Stack>
            </Stack>
          )}
        </Paper>
      </FormProvider>
    </Box>
  );
}
