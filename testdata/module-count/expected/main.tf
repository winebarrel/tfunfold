
output "first" {
  value = module.foo_0.out
}

moved {
  from = module.foo[0]
  to   = module.foo_0
}
module "foo_0" {
  source = "./modules/foo"
  name   = "f-0"
}

moved {
  from = module.foo[1]
  to   = module.foo_1
}
module "foo_1" {
  source = "./modules/foo"
  name   = "f-1"
}
