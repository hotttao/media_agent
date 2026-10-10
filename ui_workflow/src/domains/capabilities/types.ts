export interface Capability {
  key: string;
  version: number;
  provider: string;
  generation_method: string;
  generation_type: string;
  execution_target: "personal_worker" | "managed_worker";
  output_media_type: string;
  inputs: {
    name: string;
    type: string;
    required?: boolean;
    media_type?: string;
    min_items?: number;
    max_items?: number;
  }[];
}
