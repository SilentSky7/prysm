
### Build

1. 配置代理：

    ``` bash
    export http_proxy="http://172.26.248.148:7897"
    export https_proxy="http://<本机IP>:<端口>"
    ```

2. 构建

    ``` bash
    bazel build //cmd/beacon-chain:beacon-chain --config=release
    bazel build //cmd/validator:validator --config=release
    ```
