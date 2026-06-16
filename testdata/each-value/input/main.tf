resource "null_resource" "x" {
  for_each = toset(["a", "b"])
  triggers = {
    val = each.value
    tpl = "v-${each.value}"
  }
}
