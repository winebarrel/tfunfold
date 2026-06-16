module "regular" {
  source = "./modules/regular"
}

resource "single_label" {
  x = 1
}

module {
  y = 2
}


moved {
  from = null_resource.x["a"]
  to   = null_resource.x_a
}
resource "null_resource" "x_a" {
}
