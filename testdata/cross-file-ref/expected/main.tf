
resource "null_resource" "ref" {
  triggers = {
    parent = null_resource.x_a.id
  }
}

moved {
  from = null_resource.x["a"]
  to   = null_resource.x_a
}
resource "null_resource" "x_a" {
  triggers = {
    label = "x-a"
    self  = null_resource.x_a.id
  }
}

moved {
  from = null_resource.x["b"]
  to   = null_resource.x_b
}
resource "null_resource" "x_b" {
  triggers = {
    label = "x-b"
    self  = null_resource.x_b.id
  }
}
