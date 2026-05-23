variable "hetzner_token" {
  description = "Hetzner Cloud API token"
  type        = string
  sensitive   = true
}

variable "server_type" {
  description = "Hetzner server type for Coder control plane"
  type        = string
  default     = "cx22"
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "ash"
}

variable "coder_version" {
  description = "Coder version tag (e.g. v2.19.0) or 'latest'"
  type        = string
  default     = "latest"
}

variable "coder_access_url" {
  description = "Public URL Coder agents dial back to. Leave empty to use server IP."
  type        = string
  default     = ""
}

variable "postgres_password" {
  description = "Password for Coder's Postgres database"
  type        = string
  sensitive   = true
  default     = "changeme"
}
