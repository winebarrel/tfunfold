module "foo" {
  for_each = toset(["a", "b"])
  source   = "./modules/foo"
  name     = each.key
}
