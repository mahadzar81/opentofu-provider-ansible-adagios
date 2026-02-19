variable "ami" {
  type    = string
  default = null
}
variable "region" {
  type = string
}
variable "instance_type" {
  type = string
}
variable "command" {
  description = "Run initial command during initial bootstrap"
  type    = list(string)
}
variable "count_instance" {
  type = number
}
variable "key_pair_id" {
  type = string
}
variable "user" {
  type = string
}
variable "private_key" {
  description = "ssh user private key"
  type        = string
  default     = "~/.ssh/id_rsa"
}
variable "backend_bucket" {
  type = string
}
variable "backend_key" {
  type = string
}
variable "backend_dynamodb_table" {
  type = string
}







