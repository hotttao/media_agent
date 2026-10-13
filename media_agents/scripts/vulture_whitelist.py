"""Explicit dynamic/public surfaces for vulture; never imported by application code.

Keep this list narrow: router/validator decorators are handled in pyproject.toml.
Schema/dataclass fields are consumed by serialization and external configuration.
ACP callbacks are invoked by the SDK via its Client protocol, including unsupported
capabilities that must return method-not-found rather than disappear.
"""

_ = object()

# ACP Client callbacks and their protocol-mandated arguments.
_.write_text_file
_.read_text_file
_.output_byte_limit
_.terminal_output
_.release_terminal
_.wait_for_terminal_exit
_.kill_terminal
_.create_elicitation
_.complete_elicitation
_.elicitation_id
_.ext_notification
_.on_connect

# SDK stop reasons and persisted lifecycle values, including future incoming values.
_.MAX_TOKENS
_.MAX_TURN_REQUESTS
_.REFUSAL
_.DELETED
_.EXPIRED

# Universal event wire fields, emitted by dataclass serialization.
_.num_turns
_.context_tokens
_.context_window
_.cost_usd
_.parent_tool_use_id

# Pydantic/TypedDict input configuration and JSON response fields.
_.model_config
_.provider_type
_.small_model
_.mcp_config
_.shell_rc
_.credentials_dir
_.keepalive
_.environment
_.mcp
_.permissions
_.streaming_deltas
_.success
_.skills_failed
_.session_store_dir
_.disable_response_storage

# Documented compatibility exports and application-state integration surface.
_.session_manager
_.SessionSink
_.SessionManager
_.AcpSessionHandler
_.load_adapter_config

# Public context-window snapshot value in the Universal Event protocol.
_.ContextGauge
