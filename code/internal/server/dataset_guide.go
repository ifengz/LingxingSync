package server

import (
	"fmt"
	"net/http"
	"strings"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/datasetapi"
)

func (s *Server) apiDownstreamProjectGuide(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeDatasetGuide(w, r) {
		return
	}
	if s.store == nil {
		errJSON(w, http.StatusInternalServerError, "配置存储未初始化")
		return
	}
	tokenID := strings.TrimSpace(r.PathValue("token_id"))
	current := s.store.Current()
	var token *config.DatasetToken
	for i := range current.DatasetAPI.Tokens {
		if current.DatasetAPI.Tokens[i].ID == tokenID {
			token = &current.DatasetAPI.Tokens[i]
			break
		}
	}
	if token == nil {
		errJSON(w, http.StatusNotFound, "下游项目不存在")
		return
	}
	markdown, err := downstreamProjectGuide(*token, current.DatasetAPI)
	if err != nil {
		errJSON(w, http.StatusConflict, err.Error())
		return
	}
	okJSON(w, map[string]any{"project_id": token.ProjectID, "token_id": token.ID, "filename": token.ProjectID + "-integration.md", "markdown": markdown})
}

func downstreamProjectGuide(token config.DatasetToken, apiConfig config.DatasetAPIConfig) (string, error) {
	var out strings.Builder
	bearer := token.Token
	if bearer == "" {
		bearer = "<历史 Token 明文不可恢复，请重新创建下游项目>"
	}
	fmt.Fprintf(&out, "# 下游项目接入说明\n\n项目：`%s`\nToken ID：`%s`\nBearer Token：`%s`\n\n", token.ProjectID, token.ID, bearer)
	out.WriteString("本说明只描述本项目已授权的数据表和店铺。Bearer 保存在本项目中，可随时从管理页查看。\n\n")
	out.WriteString("## 授权范围\n\n数据表：\n")
	for _, datasetID := range token.DatasetScopes {
		definition, ok := datasetapi.DefinitionFor(datasetID)
		if !ok {
			return "", fmt.Errorf("数据表不可用: %s", datasetID)
		}
		if _, ok := datasetapi.SchemaFor(datasetID); !ok {
			return "", fmt.Errorf("数据表没有固定 SQL 结构: %s", datasetID)
		}
		fields := configuredDatasetFields(apiConfig, definition)
		if len(fields) == 0 {
			return "", fmt.Errorf("数据表尚未发布字段: %s", datasetID)
		}
		fmt.Fprintf(&out, "- `%s`：%s（%s，来源：`%s`，粒度：`%s`）\n", datasetID, definition.Name, definition.Kind, definition.Source, definition.Grain)
	}
	out.WriteString("\n店铺 SID：")
	if len(token.StoreScopes) == 0 {
		out.WriteString("无")
	} else {
		out.WriteString("`" + strings.Join(token.StoreScopes, "`, `") + "`")
	}
	out.WriteString("\n\n## 固定规则\n\n- 数据表版本不可变；已发布版本不能加、减、改名或改类型字段。字段变化必须注册新的数据表版本，例如 `listing-daily-v2`。\n- 下游按下面的固定 SQL 建表，接口只返回授权数据表和店铺。服务端不会连接或修改下游数据库。\n- 快照完成后保存响应中的 `changes_cursor`，后续用它调用 `/changes`；分页时持续使用 `next_cursor`。\n\n")
	for _, datasetID := range token.DatasetScopes {
		definition, _ := datasetapi.DefinitionFor(datasetID)
		schema, _ := datasetapi.SchemaFor(datasetID)
		fields := configuredDatasetFields(apiConfig, definition)
		ddl, err := schema.CreateTableSQL(fields)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "## `%s`\n\n字段：`%s`\n\n```sql\n%s\n```\n\n", datasetID, strings.Join(append(append([]string(nil), definition.FixedFields...), fields...), "`, `"), ddl)
		if datasetID == "fba-inventory-snapshot-v1" {
			out.WriteString("说明：历史从本版本部署后每次成功同步开始累计；部署前没有逐日证据，不补造旧日期。\n\n")
		}
	}
	out.WriteString("## API 示例\n\n")
	fmt.Fprintf(&out, "```http\nAuthorization: Bearer %s\nContent-Type: application/json\n```\n\n", bearer)
	for _, datasetID := range token.DatasetScopes {
		definition, _ := datasetapi.DefinitionFor(datasetID)
		fields := configuredDatasetFields(apiConfig, definition)
		store := "<STORE_SID>"
		if len(token.StoreScopes) > 0 {
			store = token.StoreScopes[0]
		}
		fmt.Fprintf(&out, "### `%s` 快照\n\n```bash\ncurl -X POST https://<LINGXING_SYNC_HOST>/api/v1/datasets/%s/snapshot \\\n  -H 'Authorization: Bearer %s' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\"store\":\"%s\",\"date_from\":\"YYYY-MM-DD\",\"date_to\":\"YYYY-MM-DD\",\"fields\":%q,\"page_size\":1000}'\n```\n\n", datasetID, datasetID, bearer, store, fields)
		fmt.Fprintf(&out, "快照最后一页返回 `changes_cursor` 后：\n\n```bash\ncurl -X POST https://<LINGXING_SYNC_HOST>/api/v1/datasets/%s/changes \\\n  -H 'Authorization: Bearer %s' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\"store\":\"%s\",\"cursor\":\"<CHANGES_CURSOR>\",\"fields\":%q,\"page_size\":1000}'\n```\n\n", datasetID, bearer, store, fields)
	}
	return strings.ReplaceAll(out.String(), "\" \"", "\",\""), nil
}
