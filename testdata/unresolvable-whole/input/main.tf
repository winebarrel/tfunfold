resource "null_resource" "x" {
  for_each = toset(["a"])
}

output "all" {
  value = null_resource.x
}
