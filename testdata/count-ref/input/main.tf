resource "null_resource" "n" {
  count    = 2
  triggers = { i = count.index }
}

resource "null_resource" "ref" {
  triggers = {
    first  = null_resource.n[0].id
    second = null_resource.n[1].id
  }
}
