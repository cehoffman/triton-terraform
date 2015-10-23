# triton-terraform

[![wercker status](https://app.wercker.com/status/ceee1ebf9da101850ac92639e6e0711d/m "wercker status")](https://app.wercker.com/project/bykey/ceee1ebf9da101850ac92639e6e0711d)

<!-- markdown-toc start - Don't edit this section. Run M-x markdown-toc-generate-toc again -->
**Table of Contents**

- [triton-terraform](#triton-terraform)
    - [Provider](#provider)
    - [Resources](#resources)
        - [`triton_key`](#tritonkey)

<!-- markdown-toc end -->

## Provider

You can set up the Triton provider for development by adding the following to
your terraform RC after `go get`ing this repo:

```hcl
providers {
  triton = "triton-terraform"
}
```

See [Terraform Plugin Basics](https://terraform.io/docs/plugins/basics.html) for
more information on installing plugins.

Then you'll need to set up the provider in a Terraform config file, like so:

```hcl
provider "triton" {
  account = "your-account-name"
  key = "~/.ssh/joyent.id_rsa" # the path to your key. If removed, defaults to ~/.ssh/id_rsa
  key_id = "50:87:72:54:cb:25:bf:af:b2:c9:61:19:59:93:fb:ab" # the corresponding key signature from your account page
}
```

## Resources

### `triton_key`

Creates and manages authentication keys in Triton. Do note that any change to
this resource, once created, will result in the old resource being destroyed and
recreated.

```hcl
resource "triton_key" "testkey" {
  name = "test key"
  key = "${file("some/other/id_rsa.pub")}"
}
```
## Using the Terraform Docker Provider

The [Terraform Docker provider](https://terraform.io/docs/providers/docker/index.html) needs to be configured with the address to the Docker API host and with the path to a directory that contains valid TLS certificates for authentication. The Docker [helper script](https://github.com/joyent/sdc-docker/tree/master/docs/api#the-helper-script) can be used to configure the Terraform provider.

Download the script:

```
curl -O https://raw.githubusercontent.com/joyent/sdc-docker/master/tools/sdc-docker-setup.sh
```

Execute the script, substituting the correct values:

```
bash sdc-docker-setup.sh <CLOUDAPI_URL> <ACCOUNT_USERNAME> ~/.ssh/<PRIVATE_KEY_FILE>
```

If you are unsure about what values to use, you can find more information in the [Docker setup script instructions](https://github.com/joyent/sdc-docker/tree/master/docs/api#the-helper-script).

The script will verify that you have the appropriate access, generate client certificates, and output some envrionment variables that you can export to configure Docker client access.

Terraform will read the values in the `DOCKER_HOST` and `DOCKER_CERT_PATH` environment variables that you generated from the script. Alternatively, you can explicitly configure the values in a Terraform provider block. For example:

```
provider "docker" {
  host = "tcp://us-east-1.docker.joyent.com:2376"
  cert_path = "/Users/localuser/.sdc/docker/jill"
}
```

