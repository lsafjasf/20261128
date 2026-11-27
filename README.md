# rendercache

一个仅使用 Go 标准库的渲染阶段缓存。它把项目、资源版本、资源内容指纹、阶段配置和上游阶段缓存键一起纳入缓存键，避免跨项目复用缓存时继续引用旧 LUT、旧字体或旧音频样本。

## 核心规则

- `ResourceRef` 由 `Project + Kind + Name + Version` 唯一确定，项目是资源身份的一部分。
- `FingerprintBytes` 对资源字节做带域的 SHA-256 指纹。注册版本时指纹由库计算，不信任调用方传入的摘要。
- 缓存键由 `project`、阶段名、阶段实现版本、配置指纹、全部直接资源指纹、全部直接上游阶段键按确定顺序编码后计算 SHA-256。
- 每个字段使用长度前缀，避免 `"ab"+"c"` 与 `"a"+"bc"` 产生相同键。
- 资源内容变化会改变直接消费该资源的阶段键；依赖该阶段的下游阶段键也会变化，但无关阶段仍可命中。
- 所有资源版本在任何缓存读取前统一解析。缺失、空版本、未知版本或无效指纹都会返回错误，并且不会执行缓存读取，也不会回退到同名资源的旧版本。
- `FileStore` 使用原子替换写单文件缓存项，并校验键、长度和载荷 SHA-256；损坏项返回 `ErrCacheEntryCorrupt`，不会被当成命中。

## 使用

```go
registry := rendercache.NewRegistry()

lut := rendercache.ResourceRef{
	Project: "film-a",
	Kind:    rendercache.KindLUT,
	Name:    "show.cube",
	Version: "2026-11-28",
}
_, err := registry.Put(rendercache.Resource{
	Ref:  lut,
	Data: lutBytes,
})
if err != nil {
	log.Fatal(err)
}

store, err := rendercache.NewFileStore("/var/cache/render")
if err != nil {
	log.Fatal(err)
}
engine := rendercache.NewEngine("film-a", registry, store)

err = engine.AddStage(rendercache.Stage{
	Name:      "color-grade",
	Version:   "3", // 实现或语义变化时必须更新
	Config:    []byte(`{"exposure":0.25}`),
	Resources: []rendercache.ResourceRef{lut},
	Run: func(ctx context.Context, input rendercache.StageInput) ([]byte, error) {
		snapshot := input.Resources[lut]
		if err := snapshot.VerifyFingerprint(); err != nil {
			return nil, err
		}
		return grade(snapshot.Data, input.Config), nil
	},
})
if err != nil {
	log.Fatal(err)
}

result, err := engine.Run(ctx)
if err != nil {
	log.Fatal(err)
}
for _, stage := range result.Stages {
	fmt.Printf("%s key=%s reused=%v\n", stage.Name, stage.Key, stage.Reused)
}
```

需要下游阶段时，在 `DependsOn` 中声明上游阶段名。`StageInput.Dependencies` 只提供直接上游的输出，缓存键也只传播直接上游键。不要通过全局变量或隐式路径读取未声明的资源，否则该输入不会进入缓存键。

## 可复现性约定

- 阶段运行函数对固定的 `StageInput` 必须确定性输出；实现或语义变化时递增 `Stage.Version`。
- `Stage.Config` 应使用稳定序列化，例如 `encoding/json` 产生的字节。原始 JSON 中键顺序或空白变化会保守地使阶段失效。
- 资源版本可以在原位置替换内容，此时指纹和所有相关阶段键都会变化。需要固定某次渲染时，持久化 `RunResult` 中的 `Manifest`，其中包含每个阶段使用的资源版本、指纹和上游键。
- `Plan` 可在不读写缓存的情况下计算并审计阶段键。

## 验证

```sh
go test ./...
go test -race ./...
go vet ./...
```
