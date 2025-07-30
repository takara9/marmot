

## oapi-codegenのインストールと、コンフィグからのコード生成

```
$ go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
$ oapi-codegen -version
github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
v2.5.0
$ oapi-codegen -config config.yaml marmot-api.yaml 
```

## APIのYAMLからHTMLページ生成

npx @redocly/cli build-docs marmot-api.yaml