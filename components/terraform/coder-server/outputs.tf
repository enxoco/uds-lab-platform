output "server_ip" {
  value = hcloud_server.coder.ipv4_address
}

output "coder_url" {
  value = local.access_url
}
