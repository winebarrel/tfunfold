output "from_target" {
  value = module.foo_a.out
}

output "from_unrelated" {
  value = module.other.out
}
