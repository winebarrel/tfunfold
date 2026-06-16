
moved {
  from = null_resource.x["a"]
  to   = null_resource.x_a
}
resource "null_resource" "x_a" {
  triggers = { k = "a" }
  lifecycle {
    create_before_destroy = true
    ignore_changes        = [triggers]
  }
}
