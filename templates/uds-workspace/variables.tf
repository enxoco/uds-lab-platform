variable "hetzner_token" {
  description = "Hetzner Cloud API token"
  type        = string
  sensitive   = true
}

variable "server_type" {
  description = "Hetzner server type for lab workspace"
  type        = string
  default     = "cx41"
  validation {
    condition     = contains(["ccx13", "ccx23", "ccx33", "cx41", "cx51"], var.server_type)
    error_message = "Must be ccx13 (8GB), ccx23 (16GB), ccx33 (32GB), cx41 (16GB), or cx51 (32GB)."
  }
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "hil"
  validation {
    condition     = contains(["ash", "hil", "nbg1", "fsn1", "hel1"], var.location)
    error_message = "Must be ash, hil, nbg1, fsn1, or hel1."
  }
}
