resource "null_resource" "n" {
  count    = 2
  triggers = { i = "n-${count.index}" }
}
