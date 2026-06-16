
moved {
  from = module.foo["a"]
  to   = module.foo_a
}
module "foo_a" {
  source = "./modules/foo"
}
