region = "ap-southeast-1"
# debian-12-amd64-20240717-1811
ami = "ami-0acbb557db23991cc"
instance_type = "c6i.xlarge"
count_instance = 1
user = "admin"
key_pair_id = "~/.ssh/id_rsa.pub"
backend_bucket = "maza-remote-state-storage-s3"
backend_key = "infrastructure/tfremote/terraform.tfstate"
backend_dynamodb_table = "terraform-state-lock-dynamo"
command             = [
    "sleep 30",
    "sudo apt update",
    "sudo apt install -y python3-minimal python3-apt python3-pip"
    ]




