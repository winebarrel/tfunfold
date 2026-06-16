
moved {
  from = null_resource.x["a"]
  to   = null_resource.x_a
}
resource "null_resource" "x_a" {
}

moved {
  from = null_resource.x["b"]
  to   = null_resource.x_b
}
resource "null_resource" "x_b" {
}
