variable "server_type" {
  description = "Hetzner server type for Coder control plane"
  type        = string
  default     = "ccx13"
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

variable "stage" {
  description = "Deployment stage (e.g. dev, prod) — injected by Atmos"
  type        = string
  default     = ""
}

variable "ssh_key_names" {
  description = "Hetzner SSH key names to inject into the server"
  type        = list(string)
  default     = []
}
