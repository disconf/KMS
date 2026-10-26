package dto

// UploadResult 是上传成功后的返回。
type UploadResult struct {
	// Key 存储标识。后续引用该文件时传 key，而不是传 URL——
	// URL 会随域名、签名策略变化，key 才是稳定标识。
	Key string `json:"key"`
	// URL 可直接访问的地址；私有文件为带签名的临时地址。
	URL string `json:"url"`
	// Name 原始文件名，仅用于回显，服务端不依赖它。
	Name string `json:"name"`
	Size int64  `json:"size"`
	MIME string `json:"mime"`
	// Private 为 true 时 URL 有有效期，过期需重新获取。
	Private bool `json:"private"`
	// ExpireAt 私有文件签名到期时间（Unix 秒）；公开文件为 0。
	ExpireAt int64 `json:"expireAt,omitempty"`
}

// UploadSceneItem 描述一个上传场景的限制，供前端做前置校验与文案提示。
type UploadSceneItem struct {
	Scene     string   `json:"scene"`
	Desc      string   `json:"desc"`
	MaxSizeMB float64  `json:"maxSizeMb"`
	Mimes     []string `json:"mimes"`
	Private   bool     `json:"private"`
	// Accept 可直接用于 <input type="file" accept="...">。
	Accept string `json:"accept"`
}
