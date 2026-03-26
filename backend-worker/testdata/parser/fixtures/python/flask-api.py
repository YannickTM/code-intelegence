"""Flask REST API for managing a collection of tasks."""

import os
import json
import logging
from datetime import datetime, timezone
from typing import Optional

from flask import Flask, Blueprint, request, jsonify, abort
from marshmallow import Schema, fields, validate

from .models import Task, db
from .auth import require_auth

__all__ = ["create_app", "TaskSchema", "TaskService"]

logger = logging.getLogger(__name__)

api = Blueprint("api", __name__, url_prefix="/api/v1")


class TaskSchema(Schema):
    """Marshmallow schema for task validation."""

    id = fields.Int(dump_only=True)
    title = fields.Str(required=True, validate=validate.Length(min=1, max=200))
    description = fields.Str(load_default="")
    completed = fields.Bool(load_default=False)
    created_at = fields.DateTime(dump_only=True)


class TaskService:
    """Service layer for task business logic."""

    def __init__(self, db_session):
        """Initialise the service with a database session."""
        self.db = db_session

    def list_tasks(self, completed: Optional[bool] = None) -> list[Task]:
        """Return all tasks, optionally filtered by completion status."""
        query = self.db.query(Task)
        if completed is not None:
            query = query.filter(Task.completed == completed)
        return query.order_by(Task.created_at.desc()).all()

    def get_task(self, task_id: int) -> Task:
        """Fetch a single task by ID or raise 404."""
        task = self.db.query(Task).get(task_id)
        if task is None:
            abort(404, description=f"Task {task_id} not found")
        return task

    def create_task(self, data: dict) -> Task:
        """Persist a new task and return it."""
        task = Task(
            title=data["title"],
            description=data.get("description", ""),
            completed=False,
            created_at=datetime.now(timezone.utc),
        )
        self.db.add(task)
        self.db.commit()
        logger.info("Created task %d: %s", task.id, task.title)
        return task

    def delete_task(self, task_id: int) -> None:
        """Remove a task by ID."""
        task = self.get_task(task_id)
        self.db.delete(task)
        self.db.commit()


schema = TaskSchema()
service: Optional[TaskService] = None


@api.route("/tasks", methods=["GET"])
def list_tasks():
    """List all tasks, with optional ?completed=true|false filter."""
    completed = request.args.get("completed")
    if completed is not None:
        completed = completed.lower() == "true"
    tasks = service.list_tasks(completed=completed)
    return jsonify(schema.dump(tasks, many=True))


@api.route("/tasks/<int:task_id>", methods=["GET"])
def get_task(task_id: int):
    """Get a single task by ID."""
    task = service.get_task(task_id)
    return jsonify(schema.dump(task))


@api.route("/tasks", methods=["POST"])
@require_auth
def create_task():
    """Create a new task from JSON body."""
    data = schema.load(request.get_json())
    task = service.create_task(data)
    return jsonify(schema.dump(task)), 201


def create_app(config_path: Optional[str] = None) -> Flask:
    """Application factory: build and return a configured Flask app."""
    app = Flask(__name__)
    if config_path:
        app.config.from_file(config_path, load=json.load)
    else:
        app.config["SQLALCHEMY_DATABASE_URI"] = os.environ.get(
            "DATABASE_URL", "sqlite:///tasks.db"
        )
    db.init_app(app)
    global service
    with app.app_context():
        service = TaskService(db.session)
    app.register_blueprint(api)
    return app
