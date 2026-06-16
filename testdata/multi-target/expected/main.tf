

moved {
  from = null_resource.a["x"]
  to   = null_resource.a_x
}
resource "null_resource" "a_x" {
}

moved {
  from = null_resource.a["y"]
  to   = null_resource.a_y
}
resource "null_resource" "a_y" {
}

moved {
  from = null_resource.b[0]
  to   = null_resource.b_0
}
resource "null_resource" "b_0" {
}

moved {
  from = null_resource.b[1]
  to   = null_resource.b_1
}
resource "null_resource" "b_1" {
}
