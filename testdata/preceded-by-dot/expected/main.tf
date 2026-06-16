
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

moved {
  from = null_resource.x["a"]
  to   = null_resource.x_a
}
resource "null_resource" "x_a" {
}
