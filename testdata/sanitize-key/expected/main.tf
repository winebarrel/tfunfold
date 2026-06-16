
moved {
  from = null_resource.x["us-east-1a"]
  to   = null_resource.x_us-east-1a
}
resource "null_resource" "x_us-east-1a" {
  triggers = { k = "us-east-1a" }
}

moved {
  from = null_resource.x["us.west.2"]
  to   = null_resource.x_us_west_2
}
resource "null_resource" "x_us_west_2" {
  triggers = { k = "us.west.2" }
}
