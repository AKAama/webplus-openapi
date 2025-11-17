# 文章api文档
## 2.0.1
### api_server
1. 增加一个articleId传参给我们自己验证用
2. 修正columnId传参的查询方式
3. 验证是某篇文章是否已存在，要考虑columnId，不能只看articleId

## 2.0.0
### 修复功能recover
1. 提供两种修复方式
   - 中间表模式
   - 查找映射文件夹
2. 支持修复指定站点或栏目
3. 