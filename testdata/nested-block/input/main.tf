resource "null_resource" "x" {
  for_each = toset(["a"])
  triggers = { k = each.key }
  lifecycle {
    create_before_destroy = true
    ignore_changes        = [triggers]
  }
}
