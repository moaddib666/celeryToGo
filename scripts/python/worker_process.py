#!/usr/bin/env python
"""
Python worker process for executing Celery tasks.

This script is executed by the Go worker to run Celery tasks in a Python environment.
It communicates with the Go worker using a JSON-based protocol over stdin/stdout.
It uses Celery's task execution mechanism for full compatibility.
"""

import json
import logging
import os
import resource
import signal
import sys
import time
import traceback
import uuid
from datetime import datetime
from typing import Dict, List, Any, Optional, Callable

# Configure logging
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s [%(levelname)s] %(message)s',
    stream=sys.stderr
)
logger = logging.getLogger('celery-go-worker')
logger.setLevel(logging.DEBUG)

# Global variables
PROCESS_ID = os.environ.get('CELERY_GO_PROCESS_ID', 'unknown')
DJANGO_SETTINGS_MODULE = os.environ.get('DJANGO_SETTINGS_MODULE')
TASKS_EXECUTED = 0
PROCESS_START_TIME = time.time()

# Message types
TASK_MESSAGE = 'task'
RESULT_MESSAGE = 'result'
PING_MESSAGE = 'ping'
PONG_MESSAGE = 'pong'
STOP_MESSAGE = 'stop'

# Task statuses
TASK_STATUS_SUCCESS = 'success'
TASK_STATUS_ERROR = 'error'


class CelerySetupError(Exception):
    """Exception raised when Celery setup fails."""
    pass


def dump_environment() -> None:
    """Dump the environment variables for debugging."""
    logger.debug("[ENV] Dumping environment variables")
    for key, value in sorted(os.environ.items()):
        if 'PASSWORD' not in key and 'SECRET' not in key:  # Don't log sensitive data
            logger.debug(f"[ENV] {key}: {value}")
    logger.debug("[ENV] Environment dump complete")


def create_message(msg_type: str, **kwargs) -> Dict[str, Any]:
    """Create a message with the given type and additional fields."""
    return {
        "type": msg_type,
        "id": f"{msg_type}-{time.time()}-{os.getpid()}",
        "timestamp": time.time(),
        **kwargs
    }


def send_message(message: Dict[str, Any]) -> None:
    """Send a message to the Go worker."""
    try:
        json_message = json.dumps(message)
        logger.debug(f"[IPC:OUT] Sending {message['type']} message")
        sys.stdout.write(json_message + '\n')
        sys.stdout.flush()
    except Exception as e:
        logger.error(f"[IPC:OUT] Failed to send message: {e}")


def send_result(task_id: str, status: str, result: Any = None, error: str = None,
                traceback_str: str = None, runtime: float = 0.0, memory_used: int = 0) -> None:
    """Send a result message to the Go worker."""
    logger.debug(f"[TASK:RESULT] Task {task_id}: status={status}, runtime={runtime:.3f}s")

    message = create_message(
        RESULT_MESSAGE,
        task_id=task_id,
        status=status,
        runtime=runtime,
        memory_used=memory_used
    )

    if result is not None:
        message["result"] = result

    if error is not None:
        message["error"] = str(error)

    if traceback_str is not None:
        message["traceback"] = traceback_str

    send_message(message)


def send_pong(ping_id: str) -> None:
    """Send a pong message in response to a ping."""
    logger.debug(f"[PING] Responding to ping {ping_id}")

    memory_usage = get_memory_usage()
    uptime = time.time() - PROCESS_START_TIME

    message = create_message(
        PONG_MESSAGE,
        ping_id=ping_id,
        memory_usage=memory_usage,
        uptime=uptime,
        tasks_executed=TASKS_EXECUTED
    )
    send_message(message)


def get_memory_usage() -> int:
    """Get the current memory usage of the process in bytes."""
    try:
        return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024
    except Exception:
        return 0


def setup_django_celery():
    """
    Set up Django and Celery environment.

    This function initializes Django and Celery based on the environment variables.
    """
    try:
        logger.debug("[SETUP] Starting Django/Celery setup")

        if DJANGO_SETTINGS_MODULE:
            logger.debug(f"[SETUP] Using Django settings module: {DJANGO_SETTINGS_MODULE}")
            import django
            os.environ.setdefault("DJANGO_SETTINGS_MODULE", DJANGO_SETTINGS_MODULE)
            django.setup()
            logger.debug("[SETUP] Django environment initialized")

        logger.debug("[SETUP] Initializing Celery")
        from celery import current_app
        app = current_app._get_current_object()

        # Log basic Celery app info
        logger.debug(f"[SETUP] Celery app initialized: {app.main}")
        logger.debug(f"[SETUP] Number of registered tasks: {len(app.tasks)}")

        # Import necessary Celery components
        from celery.worker.request import Request
        from celery.signals import task_received, task_success, task_failure

        # Import Kombu components for message handling
        from kombu.message import Message

        logger.debug("[SETUP] Celery and Kombu components imported")

        # Log registered tasks
        task_count = len(app.tasks)
        logger.debug(f"[SETUP] {task_count} tasks registered in Celery app")

        # Only log a sample of tasks to avoid cluttering logs
        sample_tasks = list(app.tasks.keys())[:5]
        for task_name in sample_tasks:
            task = app.tasks[task_name]
            is_bound = getattr(task, 'bind', False)
            logger.debug(f"[SETUP] Task: {task_name} (bound: {is_bound})")

        logger.debug("[SETUP] Celery setup complete")
        return app, Request, Message

    except Exception as e:
        logger.error(f"[SETUP] Failed to set up Django/Celery: {e}")
        logger.error(traceback.format_exc())
        raise CelerySetupError(f"Failed to set up Django/Celery: {e}")


def handle_task(request):
    """Execute a task using Celery's request object."""
    global TASKS_EXECUTED

    start_time = time.time()
    start_memory = get_memory_usage()

    logger.debug(f"[TASK:EXECUTE] Executing task {request.name} (ID: {request.id})")

    try:
        # Execute the task
        result = request.execute()

        # Calculate resource usage
        runtime = time.time() - start_time
        memory_used = get_memory_usage() - start_memory

        logger.debug(f"[TASK:SUCCESS] Task {request.id} completed in {runtime:.3f}s")
        TASKS_EXECUTED += 1

        # Send success result
        send_result(
            task_id=request.id,
            status=TASK_STATUS_SUCCESS,
            result=result,
            runtime=runtime,
            memory_used=memory_used
        )

    except Exception as e:
        # Handle task failure
        tb = traceback.format_exc()

        # Calculate resource usage
        runtime = time.time() - start_time
        memory_used = get_memory_usage() - start_memory

        logger.error(f"[TASK:ERROR] Task {request.id} failed: {e}")
        logger.debug(f"[TASK:ERROR] Traceback: {tb}")
        TASKS_EXECUTED += 1

        # Send error result
        send_result(
            task_id=request.id,
            status=TASK_STATUS_ERROR,
            error=str(e),
            traceback_str=tb,
            runtime=runtime,
            memory_used=memory_used
        )


def create_kombu_message(app, Message, task_data):
    """
    Create a Kombu Message object from task data.

    Args:
        app: Celery app
        Message: Kombu Message class
        task_data: Task data from Go worker

    Returns:
        A Kombu Message object
    """
    # Extract task information - support both possible formats
    task_name = task_data.get('task') or task_data.get('task_name')
    task_id = task_data.get('id') or task_data.get('task_id')
    args = task_data.get('args', [])
    kwargs = task_data.get('kwargs', {})
    retries = task_data.get('retries', 0)
    queue_name = task_data.get('queue', 'celery')

    # Generate ID if not provided
    if not task_id:
        task_id = str(uuid.uuid4())

    logger.debug(f"[TASK:MESSAGE] Creating Kombu message for task {task_name} (ID: {task_id})")

    # Create message properties according to protocol
    properties = {
        'correlation_id': task_id,
        'content_type': 'application/json',
        'content_encoding': 'utf-8',
    }

    # Add optional reply_to if present
    reply_to = task_data.get('reply_to')
    if reply_to:
        properties['reply_to'] = reply_to

    # Delivery info is needed for the message constructor but not part of the protocol
    delivery_info = {
        'exchange': "go-worker-exchange",
        'routing_key': queue_name,
        'priority': 0,
    }

    # Create message headers according to protocol
    headers = {
        'lang': 'py',
        'task': task_name,
        'id': task_id,
        'root_id': task_id,
        'parent_id': None,
        'group': None,
    }

    # Add optional header fields
    headers.update({
        'argsrepr': repr(args),
        'kwargsrepr': repr(kwargs),
        'origin': f'go-worker-{PROCESS_ID}',
        'retries': retries,
        'eta': None,
        'expires': None,
        'timelimit': [None, None],
        'meth': task_data.get('meth'),
        'shadow': task_data.get('shadow'),
        'replaced_task_nesting': task_data.get('replaced_task_nesting'),
    })

    # Remove None values from headers
    headers = {k: v for k, v in headers.items() if v is not None}

    # Create body according to protocol
    embed = {
        'callbacks': task_data.get('callbacks', []),
        'errbacks': task_data.get('errbacks', []),
        'chain': task_data.get('chain', []),
        'chord': task_data.get('chord'),
    }

    body = (args, kwargs, embed)

    # Create the Kombu message
    message = Message(
        body=body,
        # channel=channel, // TODO: add support for channel but not for now
        delivery_tag=None,
        properties=properties,
        headers=headers,
        delivery_info=delivery_info,
        content_type='application/json',
        content_encoding='utf-8',
    )

    # Add methods for acknowledgment/rejection that Celery's Request will call
    def ack(*args, **kwargs):
        logger.debug(f"[TASK:ACK] Task {task_id} acknowledged")

    def reject(*args, **kwargs):
        logger.debug(f"[TASK:REJECT] Task {task_id} rejected")

    message.ack = ack
    message.reject = reject
    message.ack_log_error = ack
    message.reject_log_error = reject

    return message


def handle_task_message(app, Request, Message, message_data):
    """
    Handle a task message from Go.

    Args:
        app: The Celery app
        Request: Celery Request class
        Message: Kombu Message class
        Channel: Kombu Channel class
        Connection: Kombu Connection class
        message_data: The message data from Go
    """
    try:
        # Extract task information
        task_name = message_data.get('task') or message_data.get('task_name')
        task_id = message_data.get('id') or message_data.get('task_id')

        logger.debug(f"[TASK:RECEIVED] Received task {task_name} (ID: {task_id})")

        # Check if task exists
        if task_name not in app.tasks:
            error_msg = f"Task {task_name} not found in task registry"
            logger.error(f"[TASK:ERROR] {error_msg}")
            send_result(task_id, TASK_STATUS_ERROR, error=error_msg)
            return

        # Get the task object
        task = app.tasks[task_name]

        # Create a Kombu message from the task data
        message = create_kombu_message(app, Message, message_data)

        # Create a Celery Request object from the message
        request = Request(
            decoded=True,
            message=message,
            app=app,
            hostname=f"go-worker-{PROCESS_ID}",
            eventer=None,
            task=task,
            connection_errors=(),
            # Celery 5.x has these additional parameters:
            on_ack=message.ack,
            on_reject=message.reject,
        )

        # Handle the task
        handle_task(request)

    except Exception as e:
        # Handle any errors during message/request creation
        tb = traceback.format_exc()
        logger.error(f"[TASK:ERROR] Failed to handle task message: {e}")
        logger.error(tb)

        # Try to extract task ID for error reporting
        task_id = message_data.get('id') or message_data.get('task_id') or "unknown"

        # Send error result
        send_result(
            task_id=task_id,
            status=TASK_STATUS_ERROR,
            error=f"Worker error: {e}",
            traceback_str=tb,
        )


def handle_message(app, Request, Message, message: Dict[str, Any]) -> None:
    """Handle a message from the Go worker."""
    msg_type = message.get("type")

    if msg_type == TASK_MESSAGE:
        handle_task_message(app, Request, Message, message)

    elif msg_type == PING_MESSAGE:
        # Respond to ping
        logger.debug("[IPC:IN] Received ping message")
        send_pong(message.get("id", ""))

    elif msg_type == STOP_MESSAGE:
        # Handle stop message
        graceful = message.get("graceful", True)
        logger.debug(f"[IPC:IN] Received stop message (graceful: {graceful})")
        sys.exit(0)

    else:
        logger.warning(f"[IPC:IN] Received unknown message type: {msg_type}")


def setup_signal_handlers() -> None:
    """Set up signal handlers for graceful shutdown."""

    def handle_signal(signum, frame):
        sig_name = signal.Signals(signum).name if hasattr(signal, 'Signals') else str(signum)
        logger.debug(f"[SIGNAL] Received signal {sig_name} ({signum}), shutting down...")
        sys.exit(0)

    signal.signal(signal.SIGTERM, handle_signal)
    signal.signal(signal.SIGINT, handle_signal)
    logger.debug("[SETUP] Signal handlers registered")


def main() -> None:
    """Main entry point for the worker process."""
    global PROCESS_START_TIME
    PROCESS_START_TIME = time.time()

    logger.info(f"[INIT] Starting worker process (PID: {os.getpid()}, ID: {PROCESS_ID})")
    logger.debug(f"[INIT] Python version: {sys.version}")
    logger.debug(f"[INIT] Working directory: {os.getcwd()}")
    logger.debug(f"[INIT] DJANGO_SETTINGS_MODULE: {DJANGO_SETTINGS_MODULE}")
    dump_environment()

    # Set up signal handlers
    setup_signal_handlers()

    try:
        # Set up Django and Celery
        app, Request, Message = setup_django_celery()

        # Main loop
        logger.info("[INIT] Worker process ready to receive tasks")
        for line in sys.stdin:
            try:
                # Parse the message
                logger.debug("[IPC:IN] Received message from stdin")
                message = json.loads(line.strip())

                # Handle the message
                handle_message(app, Request, Message, message)
            except json.JSONDecodeError:
                logger.error(f"[IPC:IN] Failed to parse message: {line.strip()[:100]}...")
            except Exception as e:
                logger.error(f"[IPC:IN] Error handling message: {e}")
                logger.error(traceback.format_exc())

    except CelerySetupError as e:
        logger.error(f"[INIT] Celery setup error: {e}")
        sys.exit(1)
    except Exception as e:
        logger.error(f"[INIT] Unexpected error: {e}")
        logger.error(traceback.format_exc())
        sys.exit(1)


if __name__ == "__main__":
    main()