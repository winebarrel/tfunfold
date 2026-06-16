module "foo" {
  count  = 2
  source = "./modules/foo"
  name   = "f-${count.index}"
}

output "first" {
  value = module.foo[0].out
}
