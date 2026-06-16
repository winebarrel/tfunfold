output "ids" {
  value = [null_resource.x["a"].id, null_resource.x["b"].id]
}
