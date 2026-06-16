resource "null_resource" "x" {
  for_each = toset(["a"])
}

resource "null_resource" "x_a" {}
