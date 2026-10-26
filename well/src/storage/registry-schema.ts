export const REGISTRY_KEY = "well.servers.v1";
export const REGISTRY_VERSION = 1;

export interface ServerRecord {
  id: string;
  name: string;
  base_url: string;
  remote_server_id?: string;
  remember_password: boolean;
  password?: string;
  created_at: string;
  updated_at: string;
  last_connected_at: string | null;
}

export interface RegistryDocument {
  version: 1;
  servers: ServerRecord[];
}

export interface RegistryReadResult {
  servers: ServerRecord[];
  errors: string[];
}
