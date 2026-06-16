variable "k" { type = string }

module "foo" {
  for_each = toset(["a"])
  source   = "./modules/foo"
}

output "y" {
  value = module.foo[var.k].out
}
