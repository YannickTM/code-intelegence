export type WizardStep = 1 | 2 | 3 | 4;

export type SSHKeyMode = "generate" | "existing";

export type KeyInfo = {
  id: string;
  name: string;
  public_key: string;
  fingerprint: string;
};

export type CreatedProject = {
  id: string;
  name: string;
};

export type WizardState = {
  step: WizardStep;

  // Step 1
  repoUrl: string;
  projectName: string;
  projectNameManuallyEdited: boolean;
  defaultBranch: string;

  // Step 2
  sshKeyMode: SSHKeyMode;
  newKeyName: string;
  existingKeyId: string;
  /** Set after SSH key is created or selected */
  resolvedKey: KeyInfo | null;

  // Step 3
  deployKeyConfirmed: boolean;

  // Step 4
  createdProject: CreatedProject | null;
  indexTriggered: boolean;
};

export type WizardAction =
  | { type: "SET_REPO_URL"; value: string }
  | { type: "SET_PROJECT_NAME"; value: string; manual: boolean }
  | { type: "SET_DEFAULT_BRANCH"; value: string }
  | { type: "SET_SSH_KEY_MODE"; mode: SSHKeyMode }
  | { type: "SET_NEW_KEY_NAME"; value: string }
  | { type: "SET_EXISTING_KEY_ID"; value: string }
  | { type: "SET_RESOLVED_KEY"; key: KeyInfo }
  | { type: "SET_DEPLOY_KEY_CONFIRMED"; value: boolean }
  | { type: "SET_CREATED_PROJECT"; project: CreatedProject }
  | { type: "SET_INDEX_TRIGGERED" }
  | { type: "GO_TO_STEP"; step: WizardStep }
  | { type: "NEXT_STEP" }
  | { type: "PREV_STEP" };

export const initialWizardState: WizardState = {
  step: 1,
  repoUrl: "",
  projectName: "",
  projectNameManuallyEdited: false,
  defaultBranch: "main",
  sshKeyMode: "generate",
  newKeyName: "",
  existingKeyId: "",
  resolvedKey: null,
  deployKeyConfirmed: false,
  createdProject: null,
  indexTriggered: false,
};

export function wizardReducer(
  state: WizardState,
  action: WizardAction,
): WizardState {
  switch (action.type) {
    case "SET_REPO_URL":
      return { ...state, repoUrl: action.value };
    case "SET_PROJECT_NAME":
      return {
        ...state,
        projectName: action.value,
        projectNameManuallyEdited: action.manual
          ? true
          : state.projectNameManuallyEdited,
      };
    case "SET_DEFAULT_BRANCH":
      return { ...state, defaultBranch: action.value };
    case "SET_SSH_KEY_MODE":
      return { ...state, sshKeyMode: action.mode };
    case "SET_NEW_KEY_NAME":
      return { ...state, newKeyName: action.value };
    case "SET_EXISTING_KEY_ID":
      return { ...state, existingKeyId: action.value };
    case "SET_RESOLVED_KEY":
      return { ...state, resolvedKey: action.key };
    case "SET_DEPLOY_KEY_CONFIRMED":
      return { ...state, deployKeyConfirmed: action.value };
    case "SET_CREATED_PROJECT":
      return { ...state, createdProject: action.project };
    case "SET_INDEX_TRIGGERED":
      return { ...state, indexTriggered: true };
    case "GO_TO_STEP":
      return { ...state, step: action.step };
    case "NEXT_STEP":
      return { ...state, step: Math.min(state.step + 1, 4) as WizardStep };
    case "PREV_STEP":
      return { ...state, step: Math.max(state.step - 1, 1) as WizardStep };
    default:
      return state;
  }
}
