# 从 Handler 注解生成 OpenAPI 契约

状态：Accepted

SQLFlow 以 Handler 注解和实际路由作为 HTTP 接口事实源，通过 `make docs` 生成 OpenAPI 包并在运行时提供 Swagger UI。项目文档只解释领域和使用方式，不手工维护完整端点表，从而避免路由、参数和响应在多份文档中漂移；代价是接口变更必须同时更新注解并通过生成检查。
