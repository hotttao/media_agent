"""Keep native notifications ordered until their downstream writes complete."""

from typing import Any

from acp.task import (
    DefaultMessageDispatcher,
    MessageDispatcher,
    MessageQueue,
    MessageStateStore,
    NotificationRunner,
    RequestRunner,
    TaskSupervisor,
)


def ordered_dispatcher(
    queue: MessageQueue,
    supervisor: TaskSupervisor,
    store: MessageStateStore,
    request_runner: RequestRunner,
    notification_runner: NotificationRunner,
) -> MessageDispatcher:
    class OrderedDispatcher(DefaultMessageDispatcher):
        async def _dispatch_notification(self, message: dict[str, Any]) -> None:
            # The SDK default starts detached tasks and acknowledges the queue early.
            # Requests remain concurrent, so user permission waits cannot block updates.
            await notification_runner(message)

    return OrderedDispatcher(
        queue=queue,
        supervisor=supervisor,
        store=store,
        request_runner=request_runner,
        notification_runner=notification_runner,
    )
