"""Translate native Claude control requests to persisted Agent user requests."""

from collections.abc import Awaitable, Callable

RequestHandler = Callable[[str, dict[str, object]], Awaitable[dict[str, object]]]


async def tool_decision(request: dict[str, object], handler: RequestHandler) -> dict[str, object]:
    name = str(request.get("tool_name") or "Tool")
    raw = request.get("input")
    arguments = raw if isinstance(raw, dict) else {}
    if name == "AskUserQuestion":
        questions = arguments.get("questions")
        answers: dict[str, str] = {}
        for question in questions if isinstance(questions, list) else []:
            if not isinstance(question, dict) or not isinstance(question.get("question"), str):
                continue
            text = question["question"]
            options = question.get("options")
            labels = (
                [str(o.get("label", "")) for o in options if isinstance(o, dict)]
                if isinstance(options, list)
                else []
            )
            response = await handler(
                "input", {"question": text + ("\n" + " / ".join(labels) if labels else "")}
            )
            answer = response.get("value")
            if not isinstance(answer, str) or not answer.strip():
                return {"behavior": "deny", "message": "No user answer was provided."}
            answers[text] = answer
        if not answers:
            return {"behavior": "deny", "message": "No supported question was provided."}
        return {"behavior": "allow", "updatedInput": {**arguments, "answers": answers}}
    command = arguments.get("command")
    response = await handler(
        "permission",
        {
            "tool_call": {
                "title": name,
                "raw_input": {"command": command} if isinstance(command, str) else {},
            },
            "options": [
                {"option_id": "allow_once", "name": "Allow once"},
                {"option_id": "reject_once", "name": "Deny"},
            ],
        },
    )
    if response.get("option_id") == "allow_once":
        return {"behavior": "allow", "updatedInput": arguments}
    return {"behavior": "deny", "message": "Tool execution denied by user."}
