
module "other" {
  source = "./modules/other"
}

moved {
  from = module.foo["a"]
  to   = module.foo_a
}
module "foo_a" {
  source = "./modules/foo"
}

moved {
  from = module.foo["b"]
  to   = module.foo_b
}
module "foo_b" {
  source = "./modules/foo"
}
