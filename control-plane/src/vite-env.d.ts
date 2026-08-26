/// <reference types="vite/client" />

declare module "*.svg?url" {
  const src: string;
  export default src;
}

interface ImportMetaEnv {
  readonly VITE_GATEWAY_URL?: string;
  readonly VITE_GOSO_ADMIN_TOKEN?: string;
  readonly VITE_GOSOCRM_API_URL?: string;
  readonly VITE_GOSOCRM_ORG_ID?: string;
  readonly VITE_GOSOCRM_ORG_TOKEN?: string;
}
