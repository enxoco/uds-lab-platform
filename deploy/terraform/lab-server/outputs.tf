output "server_ip" {
  value = hcloud_server.lab_server.ipv4_address
}

output "lab_url" {
  value = local.access_url
}
