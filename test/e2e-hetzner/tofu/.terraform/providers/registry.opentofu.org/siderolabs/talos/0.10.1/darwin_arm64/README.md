# terraform-provider-talos

## Debugging

In a bash shell, build a debug version of this provider binary:

```bash
make build-debug
```

In Visual Studio Code, [start the provider in a debug session](https://developer.hashicorp.com/terraform/plugin/debugging#starting-a-provider-in-debug-mode).

In a new bash shell, go into your terraform project directory, and run
terraform with `TF_REATTACH_PROVIDERS` set to the value printed in the VSCode debug windows.
