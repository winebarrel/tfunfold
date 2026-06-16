resource "null_resource" "x" {
  for_each = toset(["us-east-1a", "us.west.2"])
  triggers = { k = each.key }
}
