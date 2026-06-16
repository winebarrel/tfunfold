resource "null_resource" "x" {
  for_each = toset(["a"])
}

locals {
  shadowed = {
    null_resource = { x = "fake" }
  }
  list = ["x", "y"]
}

output "indirect" {
  value = local.shadowed.null_resource.x
}

output "by_index" {
  value = local.list.0
}
