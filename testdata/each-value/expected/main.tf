
moved {
  from = null_resource.x["a"]
  to   = null_resource.x_a
}
resource "null_resource" "x_a" {
  triggers = {
    val = "a"
    tpl = "v-a"
  }
}

moved {
  from = null_resource.x["b"]
  to   = null_resource.x_b
}
resource "null_resource" "x_b" {
  triggers = {
    val = "b"
    tpl = "v-b"
  }
}
