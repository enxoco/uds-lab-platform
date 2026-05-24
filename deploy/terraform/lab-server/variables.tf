variable "server_type" {
  description = "Hetzner server type for lab control plane"
  type        = string
  default     = "ccx13"
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "hil"
}

variable "ssh_key_names" {
  description = "Hetzner SSH key names"
  type        = list(string)
  default     = ["local"]
}

variable "domain" {
  description = "Public domain for the lab server (e.g. labs.example.com). Empty = use IP."
  type        = string
  default     = ""
}

variable "session_ttl" {
  description = "Lab session timeout in minutes"
  type        = number
  default     = 60
}

variable "hcloud_token_for_vms" {
  description = "Hetzner API token the lab server uses to provision ephemeral lab VMs"
  type        = string
  sensitive   = true
}
