# 文章api文档
## 3.1.0
### recover&server
1. 增加栏目、站点接口
2. 增加访问量字段



## 3.0.1
### recover
1. 修复domainName含有多个的情况，逗号分割


## 3.0.0
### server&recover
1. 改为mysql8存储
2. 支持配置需要模糊搜索的字段
3. 支持普通页码分页
4. 响应中增加total参数


## 2.0.2
### server&recover
1. 增加扩展字段


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