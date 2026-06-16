
moved {
  from = module.foo["a"]
  to   = module.foo_a
}
module "foo_a" {
  source = "./modules/foo"
  name   = "a"
}

moved {
  from = module.foo["b"]
  to   = module.foo_b
}
module "foo_b" {
  source = "./modules/foo"
  name   = "b"
}
