output "from_target" {
  value = module.foo["a"].out
}

output "from_unrelated" {
  value = module.other.out
}
