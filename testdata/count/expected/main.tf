
moved {
  from = null_resource.n[0]
  to   = null_resource.n_0
}
resource "null_resource" "n_0" {
  triggers = { i = "n-0" }
}

moved {
  from = null_resource.n[1]
  to   = null_resource.n_1
}
resource "null_resource" "n_1" {
  triggers = { i = "n-1" }
}
