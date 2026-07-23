

from adapter.biz.domain.acp.acp_abs import BridgeFactory, AgentHooks
from adapter.biz.domain.acp.entity import Chat
from adapter.biz.domain.acp.session import AcpSession


class AcpServer:
    """对标 AcpApp: 一个 ACP 应用，管理多个 session。"""

    def __init__(self):
        self.sessions: dict[str, AcpSession] = {}
        self.bridge_active: dict[str, int] = {}
        self.busy_turns: dict[str, int] = {}

    def __init__(self, agent_hook: AgentHooks, bridge_factory: BridgeFactory):
        self.agent_hook = agent_hook
        self.bridge_factory = bridge_factory


    def chat(self, chat: Chat):
        session_id = chat.session_id
        current_session_id = None
        # 检查会话是否存在，是否存活

        # 创建新会话
        
        bridge = self.bridge_factory(session_id)
        self.sessions[session_id] = session
        session = bridge.create_session()

        # 启动会话
        session.run_turn()
        
        return session.read()


    def reconnect():
        pass