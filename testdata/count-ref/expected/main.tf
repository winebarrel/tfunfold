
resource "null_resource" "ref" {
  triggers = {
    first  = null_resource.n_0.id
    second = null_resource.n_1.id
  }
}

moved {
  from = null_resource.n[0]
  to   = null_resource.n_0
}
resource "null_resource" "n_0" {
  triggers = { i = 0 }
}

moved {
  from = null_resource.n[1]
  to   = null_resource.n_1
}
resource "null_resource" "n_1" {
  triggers = { i = 1 }
}
