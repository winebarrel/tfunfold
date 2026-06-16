resource "null_resource" "x" {
  for_each = toset(["a"])
}

resource "null_resource" "ref" {
  triggers = { p = null_resource.x["nope"].id }
}
