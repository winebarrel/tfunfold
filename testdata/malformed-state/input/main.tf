module "foo" {
  for_each = toset(["a"])
  source   = "./modules/foo"
}
