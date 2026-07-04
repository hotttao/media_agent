package workflow

import (
	"context"
	"fmt"
	"sort"

	domain "media_agent/media_workflow/biz/domain/workflow"
)

// validator 闸1 校验器（纯逻辑 + template lookup）。
type validator struct {
	scope          string
	steps          []domain.Step
	outputStepKey  string
	templateLookup domain.TemplateLookup
}

func newValidator(scope string, steps []domain.Step, outputStepKey string, tl domain.TemplateLookup) *validator {
	return &validator{scope: scope, steps: steps, outputStepKey: outputStepKey, templateLookup: tl}
}

func (v *validator) validate(ctx context.Context) (*domain.ValidationResult, error) {
	res := &domain.ValidationResult{OutputStepKey: v.outputStepKey}

	// A. DAG 结构
	if err := v.validateDAG(res); err != nil {
		return res, nil // reject_reasons 已填
	}

	// D+F. template 引用 + scope（需 lookup）
	if err := v.validateTemplates(ctx, res); err != nil {
		return res, nil
	}

	// B. slot/binding 合法性
	if err := v.validateBindings(res); err != nil {
		return res, nil
	}

	// C. input_schema 派生
	res.InputSchema = v.deriveInputSchema()

	res.Valid = len(res.RejectReasons) == 0
	return res, nil
}

// A. DAG 结构校验
func (v *validator) validateDAG(res *domain.ValidationResult) error {
	if len(v.steps) == 0 {
		res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: "no steps"})
		return fmt.Errorf("no steps")
	}
	// step_key 唯一 + order
	keySet := make(map[string]int) // key → index
	for i, s := range v.steps {
		if s.Key == "" {
			res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: "step key is empty", Subject: fmt.Sprintf("step_order_%d", s.Order)})
			return fmt.Errorf("empty step key")
		}
		if _, exists := keySet[s.Key]; exists {
			res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: "duplicate step key", Subject: s.Key})
			return fmt.Errorf("duplicate step key %s", s.Key)
		}
		keySet[s.Key] = i
	}
	// output_step_key 存在
	if _, ok := keySet[v.outputStepKey]; !ok {
		res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: "output_step_key not found", Subject: v.outputStepKey})
		return fmt.Errorf("output_step_key %s not found", v.outputStepKey)
	}
	// 收集 step_output 依赖 + 推导 edges
	deps := make(map[string][]string) // step.key → deps (前驱)
	usedByOthers := make(map[string]bool)
	for _, s := range v.steps {
		deps[s.Key] = nil
	}
	for i, s := range v.steps {
		for _, b := range s.Bindings {
			if b.Source == domain.SourceStepOutput {
				refIdx, ok := keySet[b.Ref]
				if !ok {
					res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: "step_output ref not found", Subject: b.Ref})
					return fmt.Errorf("ref %s not found", b.Ref)
				}
				if refIdx >= i {
					res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: "step can only reference previous step", Subject: s.Key})
					return fmt.Errorf("back ref from %s to %s", s.Key, b.Ref)
				}
				deps[s.Key] = append(deps[s.Key], b.Ref)
				usedByOthers[b.Ref] = true
				res.DerivedEdges = append(res.DerivedEdges, domain.DerivedEdge{From: b.Ref, To: s.Key, ToInput: b.ToInput})
			}
		}
	}
	// 单 sink：只有 outputStepKey 不被任何人 ref
	var sinks []string
	for _, s := range v.steps {
		if !usedByOthers[s.Key] {
			sinks = append(sinks, s.Key)
		}
	}
	if len(sinks) != 1 || sinks[0] != v.outputStepKey {
		res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: fmt.Sprintf("must have exactly one output step, got %v", sinks)})
		return fmt.Errorf("multiple sinks %v", sinks)
	}
	// 无离群节点：从 outputStepKey 反向 DFS 覆盖全部
	visited := make(map[string]bool)
	var dfs func(key string)
	dfs = func(key string) {
		if visited[key] {
			return
		}
		visited[key] = true
		for _, d := range deps[key] {
			dfs(d)
		}
	}
	dfs(v.outputStepKey)
	for _, s := range v.steps {
		if !visited[s.Key] {
			res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "DAG_INVALID", Message: "orphan step", Subject: s.Key})
			return fmt.Errorf("orphan step %s", s.Key)
		}
	}
	// roots（无 STEP_OUTPUT 依赖的 step）
	for _, s := range v.steps {
		if len(deps[s.Key]) == 0 {
			res.Roots = append(res.Roots, s.Key)
		}
	}
	return nil
}

// D+F. template 引用 + scope 校验
func (v *validator) validateTemplates(ctx context.Context, res *domain.ValidationResult) error {
	for _, s := range v.steps {
		snap, err := v.templateLookup.GetVersion(ctx, s.TemplateID, s.TemplateVersion)
		if err != nil {
			res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "TEMPLATE_NOT_FOUND", Message: err.Error(), Subject: s.TemplateID})
			return fmt.Errorf("template %s v%d: %w", s.TemplateID, s.TemplateVersion, err)
		}
		if snap.Status == "archived" {
			res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "TEMPLATE_ARCHIVED", Message: "template archived", Subject: s.TemplateID})
			return fmt.Errorf("template %s archived", s.TemplateID)
		}
		if snap.Scope != v.scope {
			res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "SCOPE_MISMATCH", Message: fmt.Sprintf("template scope %q != workflow scope %q", snap.Scope, v.scope), Subject: s.TemplateID})
			return fmt.Errorf("scope mismatch for template %s", s.TemplateID)
		}
	}
	return nil
}

// B. slot/binding 合法性
func (v *validator) validateBindings(res *domain.ValidationResult) error {
	// 加载所有 template slots（已 validateTemplates 通过，再查一次）
	// 为简化：每个 step 的 binding to_input 必须在 template inputs 里；source 互斥已在 DB CHECK 保证
	// 这里做 to_input 存在性 + resource_type 自洽检查
	for _, s := range v.steps {
		// to_input 校验需要 template slots；validator 没有 ctx 之外的缓存。
		// 简化：binding 三选一互斥已由 DB CHECK 保证；to_input 存在性在 execution 时兜底。
		// TODO: P3 补充 to_input → template slot 的校验（需缓存 template snapshots）。
		for _, b := range s.Bindings {
			switch b.Source {
			case domain.SourceWorkflowInput:
				if b.Ref != "" {
					res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "BINDING_INVALID", Message: "WORKFLOW_INPUT binding ref must be empty", Subject: s.Key})
					return fmt.Errorf("workflow_input ref non-empty")
				}
			case domain.SourceStepOutput:
				if b.Ref == "" {
					res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "BINDING_INVALID", Message: "STEP_OUTPUT binding ref required", Subject: s.Key})
					return fmt.Errorf("step_output ref empty")
				}
			case domain.SourceConst:
				if b.ConstValue == "" {
					res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "BINDING_INVALID", Message: "CONST binding const_value required", Subject: s.Key})
					return fmt.Errorf("const value empty")
				}
			default:
				res.RejectReasons = append(res.RejectReasons, domain.RejectReason{Code: "BINDING_INVALID", Message: "unknown source", Subject: s.Key})
				return fmt.Errorf("unknown source")
			}
		}
	}
	return nil
}

// C. input_schema 派生：从所有 WORKFLOW_INPUT bindings 取 slot.name + resource_type + required。
// 需 template slots（resource_type/required 在 template 上，不在 binding 上）。
// 简化：本期 input_schema 仅从 binding 推 name + required（resource_type 需查 template，P3 补全）。
func (v *validator) deriveInputSchema() []domain.InputSchemaEntry {
	seen := make(map[string]*domain.InputSchemaEntry)
	for _, s := range v.steps {
		for _, b := range s.Bindings {
			if b.Source != domain.SourceWorkflowInput {
				continue
			}
			// to_input = slot.name = workflow_input name
			e, ok := seen[b.ToInput]
			if !ok {
				e = &domain.InputSchemaEntry{Name: b.ToInput, Required: true}
				seen[b.ToInput] = e
			}
			// resource_type 需从 template slot 查（P3 补全）；暂留空
			_ = s
		}
	}
	out := make([]domain.InputSchemaEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
