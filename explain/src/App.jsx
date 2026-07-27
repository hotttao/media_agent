import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  BookOpen,
  Database,
  KeyRound,
  Link2,
  Network,
  Route,
  Search,
  Server,
  Workflow,
} from "lucide-react";
import { domains, edges, tables, walkthroughs } from "./schemaData.js";

const GRAPH_NODE_WIDTH = 185;
const GRAPH_NODE_HEIGHT = 84;

const modules = [
  { id: "schema", label: "数据库表", icon: Database },
  { id: "oauth", label: "OAuth 登录", icon: KeyRound },
  { id: "mcpOauth", label: "MCP OAuth", icon: Link2 },
  { id: "agentStartup", label: "Agent 启动", icon: Server },
  { id: "configLoad", label: "Config 加载", icon: Workflow },
  { id: "skillUpdates", label: "Skill 更新", icon: Workflow },
  { id: "reading", label: "源码阅读", icon: BookOpen },
];

const missingTableGroups = [
  {
    title: "启动/运行态外围表",
    tables: ["sessions", "session_events", "messages", "agent_requests", "service_tokens", "workspace_profile", "workspace_layout"],
    note: "影响运行记录、会话、内部服务鉴权或 UI 布局，不是 agent-facing config 的第一跳。",
  },
  {
    title: "授权/共享表",
    tables: ["provider_grants", "prompt_grants", "skill_grants", "template_grants", "shares", "teams", "team_members"],
    note: "用于资源可见性和团队共享。理解 owner/visibility 时需要看，启动主线先不展开。",
  },
  {
    title: "模板扩展表",
    tables: ["template_version_schedules", "template_version_commands", "schedules", "workspace_commands"],
    note: "模板还可以携带计划任务和命令，和 agent 启动配置相关但不是每次启动都读取。",
  },
  {
    title: "记忆/通知/标签",
    tables: ["memory_stores", "memory_versions", "workspace_memory_attachments", "notifications", "workspace_tags", "workspace_tag_assignments"],
    note: "属于产品功能扩展。后续做完整源码阅读站点时可以单独做模块。",
  },
];
const moduleDomainMap = {
  schema: "all",
  oauth: "oauth",
  mcpOauth: "mcp",
  agentStartup: "config",
  configLoad: "config",
  skillUpdates: "skills",
};

const nodePositions = {
  users: [70, 60],
  oauth_clients: [300, 55],
  oauth_authorization_codes: [555, 55],
  oauth_refresh_tokens: [795, 55],
  workspaces: [70, 230],
  workspace_config: [300, 230],
  model_providers: [555, 215],
  prompts: [795, 215],
  mcp_catalog: [70, 400],
  mcp_oauth_clients: [300, 400],
  mcp_oauth_tokens: [555, 400],
  session_tokens: [795, 400],
  skill_sources: [70, 570],
  skills: [300, 570],
  skill_versions: [555, 570],
  workspace_skills: [795, 570],
  user_credentials: [70, 735],
  user_credential_workspaces: [300, 735],
  workspace_usage_events: [555, 735],
  workspace_usage_cursor: [795, 735],
};

export function App() {
  const [moduleId, setModuleId] = useState("schema");
  const [domain, setDomain] = useState("all");
  const [selected, setSelected] = useState("workspace_config");
  const [query, setQuery] = useState("");
  const [showAllFields, setShowAllFields] = useState(false);

  const activeDomain = moduleId === "schema" ? domain : moduleDomainMap[moduleId] ?? "all";

  function switchModule(next) {
    setModuleId(next);
    const nextDomain = next === "schema" ? domain : moduleDomainMap[next] ?? "all";
    const first = Object.keys(tables).find((name) => tableVisible(name, nextDomain));
    if (first && !tableVisible(selected, nextDomain)) setSelected(first);
  }

  return (
    <div className="shell">
      <Header moduleId={moduleId} setModuleId={switchModule} />
      <main className="workspace">
        {moduleId === "schema" && (
          <SchemaModule
            domain={domain}
            setDomain={setDomain}
            activeDomain={activeDomain}
            selected={selected}
            setSelected={setSelected}
            query={query}
            setQuery={setQuery}
            showAllFields={showAllFields}
            setShowAllFields={setShowAllFields}
          />
        )}
        {moduleId === "oauth" && (
          <WalkthroughModule
            kind="oauth"
            title={walkthroughs.oauthLogin.title}
            intro={walkthroughs.oauthLogin.intro}
            steps={walkthroughs.oauthLogin.steps}
            selected={selected}
            setSelected={setSelected}
            showAllFields={showAllFields}
            setShowAllFields={setShowAllFields}
          />
        )}
        {moduleId === "mcpOauth" && (
          <McpOauthModule
            selected={selected}
            setSelected={setSelected}
            showAllFields={showAllFields}
            setShowAllFields={setShowAllFields}
          />
        )}
        {moduleId === "agentStartup" && (
          <WalkthroughModule
            kind="agent"
            title={walkthroughs.agentStartup.title}
            intro={walkthroughs.agentStartup.intro}
            steps={walkthroughs.agentStartup.steps}
            selected={selected}
            setSelected={setSelected}
            showAllFields={showAllFields}
            setShowAllFields={setShowAllFields}
          />
        )}
        {moduleId === "configLoad" && (
          <WalkthroughModule
            kind="agent"
            title={walkthroughs.workspaceConfigLoad.title}
            intro={walkthroughs.workspaceConfigLoad.intro}
            steps={walkthroughs.workspaceConfigLoad.steps}
            selected={selected}
            setSelected={setSelected}
            showAllFields={showAllFields}
            setShowAllFields={setShowAllFields}
          />
        )}        {moduleId === "skillUpdates" && (
          <WalkthroughModule
            kind="skills"
            title={walkthroughs.skillUpdates.title}
            intro={walkthroughs.skillUpdates.intro}
            steps={walkthroughs.skillUpdates.steps}
            selected={selected}
            setSelected={setSelected}
            showAllFields={showAllFields}
            setShowAllFields={setShowAllFields}
          />
        )}
        {moduleId === "reading" && <ReadingModule />}
      </main>
    </div>
  );
}

function Header({ moduleId, setModuleId }) {
  return (
    <header className="topbar">
      <div>
        <div className="eyebrow">Agent System Explain</div>
        <h1>从人的观察视角理解 control-plane 到 agent 启动</h1>
        <p>
          每个模块都按“动作 → 参数 → 数据库变化 → 下一步如何使用”组织，便于边看源码边跟踪数据库里的值。
        </p>
      </div>
      <nav className="module-tabs" aria-label="模块切换">
        {modules.map((m) => {
          const Icon = m.icon;
          return (
            <button key={m.id} className={moduleId === m.id ? "active" : ""} onClick={() => setModuleId(m.id)}>
              <Icon size={16} />
              <span>{m.label}</span>
            </button>
          );
        })}
      </nav>
    </header>
  );
}

function SchemaModule(props) {
  const chooseDomain = (nextDomain) => {
    props.setDomain(nextDomain);
    if (!tableVisible(props.selected, nextDomain)) {
      const first = Object.keys(tables).find((name) => tableVisible(name, nextDomain));
      if (first) props.setSelected(first);
    }
  };
  return (
    <div className="three-col">
      <TableSidebar {...props} />
      <section className="canvas-card">
        <DomainFilters domain={props.domain} setDomain={chooseDomain} />
        <RelationGraph activeDomain={props.activeDomain} selected={props.selected} setSelected={props.setSelected} />
        <ExampleStrip />
        <MissingTableReview />
      </section>
      <TableDetails
        tableName={props.selected}
        activeDomain={props.activeDomain}
        setSelected={props.setSelected}
        showAllFields={props.showAllFields}
        setShowAllFields={props.setShowAllFields}
      />
    </div>
  );
}

function TableSidebar({ activeDomain, selected, setSelected, query, setQuery }) {
  const visibleTables = useMemo(() => {
    const q = query.trim().toLowerCase();
    return Object.keys(tables).filter((name) => {
      if (!tableVisible(name, activeDomain)) return false;
      if (!q) return true;
      const t = tables[name];
      return (
        name.toLowerCase().includes(q) ||
        t.title.toLowerCase().includes(q) ||
        t.purpose.toLowerCase().includes(q) ||
        t.fields.some((f) => f.join(" ").toLowerCase().includes(q))
      );
    });
  }, [activeDomain, query]);

  return (
    <aside className="side-card">
      <label className="search-box">
        <Search size={15} />
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索表 / 字段 / 说明" />
      </label>
      <div className="table-list">
        {visibleTables.map((name) => (
          <button key={name} className={selected === name ? "selected" : ""} onClick={() => setSelected(name)}>
            <span className="mono">{name}</span>
            <small>{tables[name].title}</small>
          </button>
        ))}
      </div>
    </aside>
  );
}

function DomainFilters({ domain, setDomain }) {
  return (
    <div className="domain-bar">
      <button className={domain === "all" ? "active" : ""} onClick={() => setDomain("all")}>全部</button>
      {domains.map((d) => (
        <button key={d.id} className={domain === d.id ? "active" : ""} onClick={() => setDomain(d.id)}>
          {d.label}
        </button>
      ))}
    </div>
  );
}

function RelationGraph({ activeDomain, selected, setSelected }) {
  const svgRef = useRef(null);
  const dragRef = useRef(null);
  const [manualPositions, setManualPositions] = useState({});
  useEffect(() => {
    setManualPositions({});
  }, [activeDomain]);
  const visibleNames = Object.keys(tables).filter((name) => tableVisible(name, activeDomain));
  const visible = new Set(visibleNames);
  const visibleEdges = edges.filter(([a, b]) => visible.has(a) && visible.has(b));
  const baseLayout = buildGraphLayout(visibleNames, activeDomain);
  const positions = Object.fromEntries(
    visibleNames.map((name) => [name, manualPositions[name] || baseLayout.positions[name]]),
  );
  const layout = { ...baseLayout, positions };
  const related = new Set(visibleEdges.filter((e) => e[0] === selected || e[1] === selected).flatMap((e) => [e[0], e[1]]));

  function svgPoint(event) {
    const svg = svgRef.current;
    if (!svg) return { x: 0, y: 0 };
    const point = svg.createSVGPoint();
    point.x = event.clientX;
    point.y = event.clientY;
    const matrix = svg.getScreenCTM();
    if (!matrix) return { x: point.x, y: point.y };
    return point.matrixTransform(matrix.inverse());
  }

  function startDrag(event, name) {
    event.preventDefault();
    setSelected(name);
    const [x, y] = positions[name];
    const point = svgPoint(event);
    dragRef.current = { name, dx: point.x - x, dy: point.y - y };
    event.currentTarget.setPointerCapture?.(event.pointerId);
  }

  function moveDrag(event) {
    if (!dragRef.current) return;
    const { name, dx, dy } = dragRef.current;
    const point = svgPoint(event);
    const nextX = Math.max(24, Math.min(layout.width - GRAPH_NODE_WIDTH - 24, point.x - dx));
    const nextY = Math.max(24, Math.min(layout.height - GRAPH_NODE_HEIGHT - 24, point.y - dy));
    setManualPositions((prev) => ({ ...prev, [name]: [nextX, nextY] }));
  }

  function endDrag() {
    dragRef.current = null;
  }

  return (
    <div className="graph-panel">
      <div className="graph-actions">
        <button type="button" onClick={() => setManualPositions({})}>重置布局</button>
      </div>
      <svg
        ref={svgRef}
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        width={layout.width}
        height={layout.height}
        role="img"
        aria-label="数据库表关系图"
        onPointerMove={moveDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
      >
        <defs>
          <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" />
          </marker>
        </defs>
        {visibleEdges.map(([from, to, label], index) => {
          const hot = from === selected || to === selected;
          const point = edgeLabelPoint(from, to, layout.positions, index);
          const labelWidth = Math.min(360, label.length * 6.6 + 36);
          return (
            <g key={`${from}-${to}-${label}`} className={hot ? "edge-group hot" : "edge-group"}>
              <path className={`edge ${hot ? "hot" : ""}`} d={edgePath(from, to, layout.positions, index)} />
              {hot && (
                <g className="edge-label-wrap" transform={`translate(${point.x}, ${point.y})`}>
                  <rect x={-labelWidth / 2} y="-13" width={labelWidth} height="25" />
                  <text className="edge-label" y="4">{label}</text>
                </g>
              )}
              <title>{`${from} -> ${to}: ${label}`}</title>
            </g>
          );
        })}
        {visibleNames.map((name) => {
          const t = tables[name];
          const [x, y] = layout.positions[name];
          const quiet = selected !== name && related.size > 0 && !related.has(name);
          return (
            <g
              key={name}
              className={`node ${selected === name ? "selected" : ""} ${quiet ? "quiet" : ""}`}
              transform={`translate(${x}, ${y})`}
              onPointerDown={(event) => startDrag(event, name)}
              onPointerMove={moveDrag}
              onPointerUp={endDrag}
              onPointerCancel={endDrag}
              onClick={() => setSelected(name)}
            >
              <rect width={GRAPH_NODE_WIDTH} height={GRAPH_NODE_HEIGHT} />
              <text x="12" y="25" className="node-name">{name}</text>
              <text x="12" y="50">{t.title}</text>
              <text x="12" y="68">{t.fields.length} fields</text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

function MissingTableReview() {
  return (
    <section className="coverage-review">
      <div>
        <strong>表覆盖 review</strong>
        <p>主图只展开 agent 启动、OAuth、MCP、Skills 的核心链路。下面这些表在 migration 里存在，但暂时按外围能力处理。</p>
      </div>
      <div className="coverage-grid">
        {missingTableGroups.map((group) => (
          <article key={group.title}>
            <h3>{group.title}</h3>
            <div className="coverage-tags">
              {group.tables.map((name) => <code key={name}>{name}</code>)}
            </div>
            <p>{group.note}</p>
          </article>
        ))}
      </div>
    </section>
  );
}
function TableDetails({ tableName, activeDomain, showAllFields, setShowAllFields, setSelected }) {
  const t = tables[tableName];
  const fields = showAllFields ? t.fields : t.fields.filter((f) => f[4]);
  const rels = edges.filter((e) => e[0] === tableName || e[1] === tableName);

  return (
    <aside className="detail-card">
      <div className="detail-head">
        <div>
          <h2 className="mono">{tableName}</h2>
          <p>{t.title}</p>
        </div>
        <button className={showAllFields ? "toggle on" : "toggle"} onClick={() => setShowAllFields(!showAllFields)}>
          {showAllFields ? "全部字段" : "关键字段"}
        </button>
      </div>
      <div className="detail-section">
        <h3>业务作用</h3>
        <p>{t.purpose}</p>
        <p className="hint">什么时候看：{t.readWhen}</p>
        {t.notes && (
          <ul className="key-notes">
            {t.notes.map((note) => <li key={note}>{note}</li>)}
          </ul>
        )}
      </div>
      <div className="detail-section">
        <h3>字段说明</h3>
        <table className="field-table">
          <thead><tr><th>字段</th><th>类型</th><th>标记</th><th>说明</th></tr></thead>
          <tbody>
            {fields.map(([name, type, marker, desc]) => (
              <tr key={name}>
                <td className="mono">{name}</td>
                <td className="mono muted">{type}</td>
                <td><Marker marker={marker} /></td>
                <td>{desc}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!showAllFields && <p className="hint">当前只显示关键字段；右上角可切换全部字段。</p>}
      </div>
      <div className="detail-section">
        <h3>相关关系</h3>
        <div className="rel-list">
          {rels.map(([from, to, label]) => {
            const other = from === tableName ? to : from;
            if (activeDomain !== "all" && !tableVisible(other, activeDomain)) return null;
            return (
              <button key={`${from}-${to}-${label}`} onClick={() => setSelected?.(other)} className="rel-item">
                <span><code>{from}</code> → <code>{to}</code></span>
                <small>{label}</small>
              </button>
            );
          })}
        </div>
      </div>
      <div className="detail-section">
        <h3>示例行</h3>
        <pre>{JSON.stringify(t.sample, null, 2)}</pre>
      </div>
    </aside>
  );
}

function Marker({ marker }) {
  if (!marker) return <span className="marker empty">-</span>;
  const kind = marker.includes("sensitive") ? "danger" : marker.includes("FK") ? "fk" : marker.includes("PK") ? "pk" : "";
  return <span className={`marker ${kind}`}>{marker}</span>;
}

function ExampleStrip() {
  return (
    <div className="example-strip">
      <div><strong>同一组 ID</strong><span>用户 <code>u_alice</code> 创建 workspace <code>ws_codex_demo</code></span></div>
      <div><strong>配置解析</strong><span><code>workspace_config</code> 引用 provider 和 prompt，CP 生成 agent-facing config</span></div>
      <div><strong>能力挂载</strong><span>Skills 走 <code>workspace_skills</code>，凭据走 selected scope，HTTP MCP 走 session 动态挂载</span></div>
    </div>
  );
}

function WalkthroughModule({ title, intro, steps, selected, setSelected, showAllFields, setShowAllFields, kind }) {
  const domain = kind === "oauth" ? "oauth" : kind === "mcp" ? "mcp" : kind === "skills" ? "skills" : "config";
  const involved = new Set(steps.flatMap((s) => s.db.map((d) => d.table)).filter(Boolean));
  const first = Array.from(involved)[0] || selected;
  const current = involved.has(selected) ? selected : first;

  return (
    <div className="walk-layout">
      <section className="walk-main">
        <div className="module-title">
          <Workflow size={21} />
          <div>
            <h2>{title}</h2>
            <p>{intro}</p>
          </div>
        </div>
        <div className="timeline">
          {steps.map((step, idx) => (
            <StepCard key={step.title} step={step} index={idx + 1} onPickTable={setSelected} />
          ))}
        </div>
      </section>
      <TableDetails
        tableName={current}
        activeDomain={domain}
        setSelected={setSelected}
        showAllFields={showAllFields}
        setShowAllFields={setShowAllFields}
      />
    </div>
  );
}

function McpOauthModule({ selected, setSelected, showAllFields, setShowAllFields }) {
  const steps = [
    {
      title: "UI 检测 MCP 是否需要 OAuth",
      actor: "Browser -> control-plane",
      request: "POST /api/mcp-oauth/discover",
      url: "http://127.0.0.1:30080/api/mcp-oauth/discover",
      params: { url: "https://docs.example.com/mcp" },
      paramNotes: [
        "url：用户在 UI 里输入或从 mcp_catalog 选择出来的 MCP endpoint。",
        "CP 会从 url 解析出 server_origin=https://docs.example.com，后续 client、token 都按 server_origin 归类。",
        "如果上游支持 OAuth discovery，CP 会读取 authorization_endpoint、token_endpoint、registration_endpoint 等 metadata，并保存到 mcp_oauth_clients.metadata。",
      ],
      response: { status: 200, body: { requires_oauth: true, server_origin: "https://docs.example.com", authorization_endpoint: "https://docs.example.com/oauth/authorize", token_endpoint: "https://docs.example.com/oauth/token", registration_endpoint: "https://docs.example.com/oauth/register" } },
      implementation: [
        "处理位置：前端连接 MCP 时调用 CP 的 /api/mcp-oauth/discover。",
        "参数来源：url 来自用户配置的 MCP server URL，不是 SaaS 管理页 saas_url。",
        "后端处理：解析 origin，尝试读取 OAuth metadata；如果需要 OAuth，就准备后续动态注册/授权。",
        "数据库处理：可能读取或准备 upsert mcp_oauth_clients，但这一步通常还没有用户 token。",
      ],
      db: [{ table: "mcp_oauth_clients", op: "SELECT/UPSERT later", values: { server_origin: "https://docs.example.com", metadata: { authorization_endpoint: "https://docs.example.com/oauth/authorize", token_endpoint: "https://docs.example.com/oauth/token" } } }],
      note: "这一步只是在判断上游 MCP 是否需要 OAuth，以及 OAuth endpoint 在哪里。catalog 里的 saas_url 只是 SaaS 页面，不是 MCP endpoint。",
    },
    {
      title: "用户点击连接，跳到上游授权页",
      actor: "Browser -> CP -> Upstream MCP OAuth",
      request: "GET /api/mcp-oauth/authorize?server_origin=...",
      url: "http://127.0.0.1:30080/api/mcp-oauth/authorize?server_origin=https%3A%2F%2Fdocs.example.com&workspace_id=ws_codex_demo",
      redirectTo: "https://docs.example.com/oauth/authorize?client_id=mcp-client-123&redirect_uri=http%3A%2F%2F127.0.0.1%3A30080%2Fapi%2Fmcp-oauth%2Fcallback&response_type=code&scope=mcp.read%20mcp.write&state=cp_pending_state&code_challenge=pkce_challenge&code_challenge_method=S256",
      params: { server_origin: "https://docs.example.com", workspace_id: "ws_codex_demo", upstream_authorize_params: { client_id: "mcp-client-123", redirect_uri: "http://127.0.0.1:30080/api/mcp-oauth/callback", response_type: "code", scope: "mcp.read mcp.write", state: "cp_pending_state", code_challenge: "pkce_challenge", code_challenge_method: "S256" } },
      paramNotes: [
        "server_origin：上游 MCP 服务源站。CP 用它查 mcp_oauth_clients，并决定向哪个 OAuth server 发起授权。",
        "workspace_id：授权完成后需要 reload 的 workspace；否则 token 保存了但 agent 不会及时拿到 proxy 配置。",
        "client_id：CP 作为第三方 OAuth client 在上游 MCP 注册得到的客户端 ID，保存在 mcp_oauth_clients.client_id。",
        "redirect_uri：上游授权完成后回调到 CP 的地址，必须和动态注册/上游配置允许的回调地址一致。",
        "response_type=code：使用 Authorization Code Flow，上游先给 code，CP 后端再换 token。",
        "scope：CP 请求访问上游 MCP 的权限范围，具体值取决于上游 MCP metadata/实现。",
        "state：CP 生成的防 CSRF 与上下文关联值，用来找回 server_origin、workspace_id、PKCE verifier。",
        "code_challenge/code_challenge_method：PKCE 参数；上游发 code 前保存 challenge，token 阶段用 code_verifier 校验。",
      ],
      response: { status: 302, headers: { Location: "https://docs.example.com/oauth/authorize?..." }, body: "empty" },
      implementation: [
        "处理位置：用户点击 Connect 后，浏览器请求 CP 的 /api/mcp-oauth/authorize。",
        "参数来源：server_origin/workspace_id 来自前端当前 MCP 配置上下文；client_id 来自 mcp_oauth_clients.client_id；authorize/token endpoint 来自 mcp_oauth_clients.metadata。",
        "后端处理：生成 state 和 PKCE verifier/challenge，把 pending 授权上下文暂存起来，然后 302 到上游 OAuth authorize endpoint。",
        "浏览器行为：浏览器离开 CP 页面，打开第三方 MCP 的授权页。",
      ],
      db: [{ table: "mcp_oauth_clients", op: "UPSERT", values: { server_origin: "https://docs.example.com", metadata: { authorization_endpoint: "https://docs.example.com/oauth/authorize", token_endpoint: "https://docs.example.com/oauth/token" }, client_id: "mcp-client-123", client_secret: "encrypted-or-null" } }],
      note: "这一段里 CP 的角色变了：CP 不是 OAuth Provider，而是第三方 MCP 的 OAuth Client。",
    },
    {
      title: "授权 callback 保存 token",
      actor: "Upstream -> Browser -> CP callback",
      request: "GET /api/mcp-oauth/callback?code=...&state=...",
      url: "http://127.0.0.1:30080/api/mcp-oauth/callback?code=upstream_code&state=cp_pending_state",
      params: { code: "upstream_code", state: "cp_pending_state", token_request: { grant_type: "authorization_code", client_id: "mcp-client-123", redirect_uri: "http://127.0.0.1:30080/api/mcp-oauth/callback", code_verifier: "pkce_original_verifier" } },
      paramNotes: [
        "code：上游 MCP OAuth server 发回的一次性授权码，只能被 CP 后端消费一次。",
        "state：CP 用它校验 callback 合法性，并找回 pending 授权里的 server_origin、workspace_id、code_verifier。",
        "grant_type=authorization_code：token 请求类型，表示用 authorization code 换 token。",
        "client_id：CP 在上游 MCP 注册得到的 client_id，必须和 authorize 阶段一致。",
        "redirect_uri：必须和 authorize 阶段传给上游的 redirect_uri 一致。",
        "code_verifier：PKCE 原文；上游会计算后和 code_challenge 比对。",
      ],
      response: { token_endpoint_response: { status: 200, body: { access_token: "upstream_access_token", refresh_token: "upstream_refresh_token", token_type: "Bearer", expires_in: 3600, scope: "mcp.read mcp.write" } }, browser_response: { status: 302, headers: { Location: "http://127.0.0.1:30080/mcp/connected?workspace_id=ws_codex_demo" }, body: "empty" } },
      implementation: [
        "处理位置：上游授权完成后，浏览器带 code/state 回到 CP 的 /api/mcp-oauth/callback。",
        "参数来源：code/state 来自上游 callback URL；code_verifier 来自 CP 发起授权时保存的 pending 上下文。",
        "后端处理：校验 state，向上游 token_endpoint 发 POST 换 token。",
        "数据库处理：把 access_token/refresh_token 按 user_id + server_origin upsert 到 mcp_oauth_tokens。",
        "收尾处理：通知 workspace reload config，通常再 302 回 CP 前端的连接完成页。",
      ],
      db: [{ table: "mcp_oauth_tokens", op: "UPSERT", values: { user_id: "u_alice", server_origin: "https://docs.example.com", access_token: "<encrypted-ish token>", refresh_token: "<refresh>" } }],
      note: "这里保存的是用户授权 CP 访问上游 MCP 的 token，不是 CP 登录 token，也不是 agent 自己的 token。",
    },
    {
      title: "agent 重新拉 config，MCP URL 被改成 CP proxy",
      actor: "Agent -> CP",
      request: "GET /_cp/workspaces/ws_codex_demo/config",
      url: "http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/config",
      params: { mcp_config: { docs: { url: "https://docs.example.com/mcp" } } },
      paramNotes: [
        "mcp_config：workspace_config 里启用的 MCP server 清单。",
        "docs.url：用户配置的上游 MCP endpoint，CP 会用它推导 server_origin。",
        "server_origin：CP 用它查 mcp_oauth_tokens，判断当前用户是否已经授权过这个上游 MCP。",
        "proxy URL：返回给 agent 的不是上游 URL，而是 CP proxy URL；agent 后续请求 CP，CP 再注入 Authorization 到上游。",
      ],
      response: { status: 200, body: { workspace_id: "ws_codex_demo", mcp_config: { docs: { url: "http://127.0.0.1:30080/api/mcp-proxy?server_origin=https%3A%2F%2Fdocs.example.com&workspace_id=ws_codex_demo", original_url: "https://docs.example.com/mcp", oauth: true } } } },
      implementation: [
        "处理位置：agent 启动或 reload 后请求 /_cp/workspaces/:id/config。",
        "参数来源：workspace_id 来自 agent 环境；mcp_config 来自 workspace_config 或 template_versions 合并结果。",
        "后端处理：遍历 mcp_config，按 server_origin 查询 mcp_oauth_tokens。",
        "返回处理：如果 token 存在，把该 MCP server URL 改写成 CP proxy；agent 不直接持有上游 access_token。",
      ],
      db: [
        { table: "workspace_config", op: "SELECT", values: { workspace_id: "ws_codex_demo", mcp_config: "{ docs url }" } },
        { table: "mcp_oauth_tokens", op: "SELECT", values: { user_id: "u_alice", server_origin: "https://docs.example.com" } },
      ],
      note: "agent 实际访问 CP proxy。CP proxy 再给上游 MCP 注入 Authorization: Bearer upstream_access_token。",
    },
  ];
  return (
    <WalkthroughModule
      kind="mcp"
      title="MCP OAuth：从连接第三方 MCP 到 agent 使用 proxy"
      intro="观察重点：mcp_catalog 只是目录；workspace_config.mcp_config 是启用清单；mcp_oauth_tokens 让 CP proxy 能代表用户访问上游 MCP。"
      steps={steps}
      selected={selected}
      setSelected={setSelected}
      showAllFields={showAllFields}
      setShowAllFields={setShowAllFields}
    />
  );
}
function StepCard({ step, index, onPickTable }) {
  return (
    <article className="step-card">
      <div className="step-index">{index}</div>
      <div className="step-body">
        <h3>{step.title}</h3>
        <div className="step-meta">
          <span><Route size={14} />{step.actor}</span>
          <span><Network size={14} />{step.request}</span>
        </div>
        {step.url && (
          <div className="url-block">
            <strong>请求 URL</strong>
            <code>{step.url}</code>
          </div>
        )}
        {step.redirectTo && (
          <div className="url-block redirect">
            <strong>跳转目标</strong>
            <code>{step.redirectTo}</code>
          </div>
        )}
        {step.params && (
          <div className="code-block">
            <strong>观察参数</strong>
            <pre>{JSON.stringify(step.params, null, 2)}</pre>
          </div>
        )}
        {step.paramNotes && (
          <div className="impl-block param-notes">
            <strong>参数作用</strong>
            <ul>
              {step.paramNotes.map((item) => <li key={item}>{item}</li>)}
            </ul>
          </div>
        )}
        {step.response && (
          <div className="code-block response-block">
            <strong>接口返回</strong>
            <pre>{typeof step.response === "string" ? step.response : JSON.stringify(step.response, null, 2)}</pre>
          </div>
        )}
        {step.implementation && (
          <div className="impl-block">
            <strong>实现处理</strong>
            <ul>
              {step.implementation.map((item) => <li key={item}>{item}</li>)}
            </ul>
          </div>
        )}
        {step.sql && (
          <div className="code-block sql-block">
            <strong>SQL 关键片段</strong>
            <pre>{step.sql}</pre>
          </div>
        )}
        <div className="db-changes">
          <strong>数据库变化</strong>
          {step.db.length === 0 ? <p className="hint">无数据库写入或读取，主要是浏览器跳转/本地内存状态。</p> : (
            step.db.map((change, i) => (
              <button key={`${change.table}-${i}`} onClick={() => onPickTable(change.table)}>
                <span className="op">{change.op}</span>
                <code>{change.table}</code>
                <pre>{JSON.stringify(change.values, null, 2)}</pre>
              </button>
            ))
          )}
        </div>
        <p className="note">{step.note}</p>
      </div>
    </article>
  );
}

function ReadingModule() {
  const sections = [
    ["路由总入口", "D:/Code/koala/agent-platform/control-plane/src/index.ts:299-351", "先看 /api、/_cp、/mcp 如何挂载。"],
    ["agent-facing config", "D:/Code/koala/agent-platform/control-plane/src/routes/internal.ts:273-360", "看 CP 如何返回 /_cp/workspaces/:id/config。"],
    ["config SQL 合并", "D:/Code/koala/agent-platform/control-plane/src/services/db/workspaces.ts:277-333", "看 template/provider/prompt 如何合并。"],
    ["Python agent 生命周期", "D:/Code/media/media_agent/media_agents/adapter/agent_lifespan.py:16-35", "看 hooks 调用顺序。"],
    ["Python Codex hooks", "D:/Code/media/media_agent/media_agents/codex/config.py:66-198", "看 config/MCP/credentials 如何落盘。"],
    ["OAuth 参考文档", "D:/Code/koala/agent-platform/docs/wiki/auth/oauth.md", "看 Sandbox/Browser 登录、JWT 和 DB 写入。"],
  ];
  return (
    <section className="reading">
      <div className="module-title">
        <BookOpen size={22} />
        <div>
          <h2>源码阅读入口</h2>
          <p>后续可以把这里扩展成完整源码阅读站点：点击文件、查看调用链、关联数据库表和示例值。</p>
        </div>
      </div>
      <div className="reading-grid">
        {sections.map(([title, path, desc]) => (
          <div className="reading-card" key={path}>
            <h3>{title}</h3>
            <code>{path}</code>
            <p>{desc}</p>
          </div>
        ))}
      </div>
    </section>
  );
}

function tableVisible(name, domain) {
  if (domain === "all") return true;
  const t = tables[name];
  return t.domain === domain || t.importantFor?.includes(domain);
}

function buildGraphLayout(names, domain) {
  const visible = new Set(names);
  const presets = {
    config: [["users"], ["workspaces"], ["workspace_config"], ["templates", "template_versions"], ["model_providers", "prompts", "prompt_versions", "template_version_skills"], ["mcp_catalog", "session_tokens", "workspace_usage_events", "workspace_usage_cursor"]],
    oauth: [["users", "oauth_clients"], ["oauth_authorization_codes"], ["oauth_refresh_tokens"]],
    mcp: [["mcp_catalog"], ["workspace_config", "mcp_oauth_clients"], ["mcp_oauth_tokens"], ["users"]],
    skills: [["skill_sources"], ["skills"], ["skill_versions", "workspace_skills", "template_version_skills"], ["workspaces"]],
    credentials: [["users"], ["user_credentials"], ["user_credential_workspaces"], ["workspaces"]],
    runtime: [["workspaces"], ["session_tokens", "workspace_usage_events"], ["workspace_usage_cursor"]],
    all: [
      ["users", "oauth_clients", "oauth_authorization_codes", "oauth_refresh_tokens"],
      ["workspaces", "workspace_config", "templates", "template_versions", "model_providers", "prompts", "prompt_versions"],
      ["mcp_catalog", "mcp_oauth_clients", "mcp_oauth_tokens", "session_tokens"],
      ["skill_sources", "skills", "skill_versions", "workspace_skills", "template_version_skills"],
      ["user_credentials", "user_credential_workspaces", "workspace_usage_events", "workspace_usage_cursor"],
    ],
  };
  const columns = (presets[domain] || presets.all)
    .map((col) => col.filter((name) => visible.has(name)))
    .filter((col) => col.length > 0);
  const placed = new Set(columns.flat());
  const leftovers = names.filter((name) => !placed.has(name));
  if (leftovers.length) columns.push(leftovers);

  const nodeW = GRAPH_NODE_WIDTH;
  const nodeH = GRAPH_NODE_HEIGHT;
  const gapX = domain === "all" ? 230 : 255;
  const gapY = domain === "all" ? 112 : 132;
  const padX = 52;
  const padY = 48;
  const maxRows = Math.max(1, ...columns.map((col) => col.length));
  const height = Math.max(360, padY * 2 + maxRows * nodeH + (maxRows - 1) * gapY);
  const positions = {};

  columns.forEach((col, colIndex) => {
    const colHeight = col.length * nodeH + (col.length - 1) * gapY;
    const startY = Math.max(padY, (height - colHeight) / 2);
    col.forEach((name, rowIndex) => {
      positions[name] = [padX + colIndex * gapX, startY + rowIndex * (nodeH + gapY)];
    });
  });

  return {
    positions,
    width: Math.max(1080, padX * 2 + columns.length * nodeW + (columns.length - 1) * gapX),
    height,
  };
}

function edgePath(from, to, positions, index = 0) {
  const [x1, y1] = positions[from] || nodePositions[from];
  const [x2, y2] = positions[to] || nodePositions[to];
  const nodeW = GRAPH_NODE_WIDTH;
  const nodeH = GRAPH_NODE_HEIGHT;
  const leftToRight = x2 >= x1;
  const sx = leftToRight ? x1 + nodeW : x1;
  const sy = y1 + nodeH / 2;
  const tx = leftToRight ? x2 : x2 + nodeW;
  const ty = y2 + nodeH / 2;
  const dx = Math.max(72, Math.abs(tx - sx) * 0.48);
  const lane = ((index % 5) - 2) * 18;
  return `M ${sx} ${sy} C ${sx + (leftToRight ? dx : -dx)} ${sy + lane}, ${tx - (leftToRight ? dx : -dx)} ${ty - lane}, ${tx} ${ty}`;
}

function edgeLabelPoint(from, to, positions, index = 0) {
  const [x1, y1] = positions[from] || nodePositions[from];
  const [x2, y2] = positions[to] || nodePositions[to];
  const lane = ((index % 5) - 2) * 18;
  return {
    x: (x1 + x2) / 2 + GRAPH_NODE_WIDTH / 2,
    y: (y1 + y2) / 2 + GRAPH_NODE_HEIGHT / 2 + lane,
  };
}







