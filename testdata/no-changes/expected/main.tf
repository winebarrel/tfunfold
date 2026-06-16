resource "null_resource" "plain" {
  triggers = { x = "y" }
}
