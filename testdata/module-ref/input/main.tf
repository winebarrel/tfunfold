module "foo" {
  for_each = toset(["a", "b"])
  source   = "./modules/foo"
}

module "other" {
  source = "./modules/other"
}
