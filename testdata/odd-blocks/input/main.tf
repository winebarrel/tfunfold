module "regular" {
  source = "./modules/regular"
}

resource "single_label" {
  x = 1
}

module {
  y = 2
}

resource "null_resource" "x" {
  for_each = toset(["a"])
}
