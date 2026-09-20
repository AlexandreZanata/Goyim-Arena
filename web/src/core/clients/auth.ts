/**
 * Authentication client (P18-T03; journey P18-T05).
 *
 * Paths mirror the versioned contract (`api/openapi.json`); the transport,
 * CSRF, timeout, cancellation and failure decoding belong to the core.
 */
import type {
  LoginRequest,
  LoginResponse,
  PasswordResetConfirm,
  PasswordResetRequest,
  PrivateProfile,
  RegisterRequest,
  RegisterResponse,
  StatusResponse,
} from "../../contracts/generated.js";
import type { HttpCore } from "../http.js";

/** Operations the anonymous and authenticated journeys need. */
export interface AuthClient {
  register(input: RegisterRequest): Promise<RegisterResponse>;
  login(input: LoginRequest): Promise<LoginResponse>;
  logout(): Promise<StatusResponse>;
  requestPasswordReset(input: PasswordResetRequest): Promise<StatusResponse>;
  confirmPasswordReset(input: PasswordResetConfirm): Promise<StatusResponse>;
  /** The authenticated account, used to decide the shell of every page. */
  session(): Promise<PrivateProfile>;
}

const AUTH_PATH = "/api/v1/auth";
const SESSION_PATH = "/api/v1/me/profile";

/** createAuthClient binds the auth operations to a shared core. */
export function createAuthClient(core: HttpCore): AuthClient {
  return {
    register: (input: RegisterRequest): Promise<RegisterResponse> =>
      core.request<RegisterResponse>({ method: "POST", path: `${AUTH_PATH}/register`, body: input }),

    // A 401 from login means "wrong credentials", not "session expired":
    // it must not tear down the page the visitor is filling in.
    login: (input: LoginRequest): Promise<LoginResponse> =>
      core.request<LoginResponse>({
        method: "POST",
        path: `${AUTH_PATH}/login`,
        body: input,
        tolerateUnauthorized: true,
      }),

    // Logging out with an already expired session is success, not failure.
    logout: (): Promise<StatusResponse> =>
      core.request<StatusResponse>({
        method: "POST",
        path: `${AUTH_PATH}/logout`,
        tolerateUnauthorized: true,
      }),

    requestPasswordReset: (input: PasswordResetRequest): Promise<StatusResponse> =>
      core.request<StatusResponse>({
        method: "POST",
        path: `${AUTH_PATH}/password-reset/request`,
        body: input,
        tolerateUnauthorized: true,
      }),

    confirmPasswordReset: (input: PasswordResetConfirm): Promise<StatusResponse> =>
      core.request<StatusResponse>({
        method: "POST",
        path: `${AUTH_PATH}/password-reset/confirm`,
        body: input,
        tolerateUnauthorized: true,
      }),

    session: (): Promise<PrivateProfile> =>
      core.request<PrivateProfile>({ method: "GET", path: SESSION_PATH, tolerateUnauthorized: true }),
  };
}
