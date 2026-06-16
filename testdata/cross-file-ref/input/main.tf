resource "null_resource" "x" {
  for_each = toset(["a", "b"])
  triggers = {
    label = "x-${each.key}"
    self  = null_resource.x[each.key].id
  }
}

resource "null_resource" "ref" {
  triggers = {
    parent = null_resource.x["a"].id
  }
}
