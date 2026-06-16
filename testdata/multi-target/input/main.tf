resource "null_resource" "a" {
  for_each = toset(["x", "y"])
}

resource "null_resource" "b" {
  count = 2
}
