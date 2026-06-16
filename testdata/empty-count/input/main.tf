resource "null_resource" "z" {
  count = 0
}
resource "null_resource" "y" {
  for_each = toset([])
}
