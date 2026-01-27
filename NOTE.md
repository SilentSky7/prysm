
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

### Windows 配置

设置GO代理：go env -w GOPROXY=https://proxy.golang.com.cn,direct
永久设置环境：setx GOPROXY "https://proxy.golang.com.cn,direct"

PS:
$env:http_proxy="http://127.0.0.1:7897"
$env:https_proxy="http://127.0.0.1:7897"

CMD:
set http_proxy=http://127.0.0.1:7897
set https_proxy=http://127.0.0.1:7897
