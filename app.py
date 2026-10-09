import argparse
import json
import logging
import os
import sqlite3
import uuid
from datetime import datetime, timezone
from pathlib import Path

from flask import Flask, jsonify, request, send_from_directory
from werkzeug.exceptions import HTTPException

DATA = Path(os.environ.get('DATA_DIR', './data'))
DATA.mkdir(parents=True, exist_ok=True)
(DATA / 'photos').mkdir(exist_ok=True)
logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s')
log = logging.getLogger('student-console')
app = Flask(__name__, static_folder='public', static_url_path='')
app.config['MAX_CONTENT_LENGTH'] = 5 * 1024 * 1024


def db():
    conn = sqlite3.connect(DATA / 'students.db', timeout=20)
    conn.row_factory = sqlite3.Row
    conn.execute('PRAGMA foreign_keys=ON')
    return conn


with db() as c:
    c.executescript('''
    PRAGMA journal_mode=WAL;
    CREATE TABLE IF NOT EXISTS students (
      id TEXT PRIMARY KEY, first_name TEXT NOT NULL, last_name TEXT NOT NULL,
      age INTEGER NOT NULL CHECK(age BETWEEN 1 AND 120), national_id TEXT NOT NULL UNIQUE,
      email TEXT NOT NULL, phone TEXT NOT NULL, photo_url TEXT, created_at TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS registrations (
      id TEXT PRIMARY KEY, student_id TEXT NOT NULL REFERENCES students(id),
      course TEXT NOT NULL, semester TEXT NOT NULL, status TEXT NOT NULL,
      fee_cents INTEGER NOT NULL CHECK(fee_cents >= 0), created_at TEXT NOT NULL,
      UNIQUE(student_id, course, semester));
    CREATE TABLE IF NOT EXISTS payments (
      id TEXT PRIMARY KEY, registration_id TEXT NOT NULL REFERENCES registrations(id),
      amount_cents INTEGER NOT NULL CHECK(amount_cents > 0), method TEXT NOT NULL,
      reference TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL);
    ''')


def now():
    return datetime.now(timezone.utc).isoformat()


def fields(names):
    body = request.get_json(silent=True)
    if not isinstance(body, dict):
        raise ValueError('Send a JSON object.')
    for name in names:
        if name not in body or body[name] is None or (isinstance(body[name], str) and not body[name].strip()):
            raise ValueError(f'{name} is required.')
    return body


def text(body, name, limit=120):
    value = body.get(name, '')
    if not isinstance(value, str) or len(value.strip()) > limit:
        raise ValueError(f'{name} must be text of at most {limit} characters.')
    return value.strip()


def integer(body, name, low, high):
    value = body[name]
    if type(value) is not int or not low <= value <= high:
        raise ValueError(f'{name} must be an integer between {low} and {high}.')
    return value


@app.errorhandler(Exception)
def error(exc):
    if isinstance(exc, HTTPException):
        return jsonify(error=exc.description), exc.code
    if isinstance(exc, ValueError):
        return jsonify(error=str(exc)), 400
    if isinstance(exc, sqlite3.IntegrityError):
        return jsonify(error='Duplicate record or invalid related record.'), 409
    log.exception('Request failed')
    return jsonify(error='Internal server error.'), 500


@app.after_request
def audit(response):
    log.info('%s %s status=%s', request.method, request.path, response.status_code)
    response.headers['X-Content-Type-Options'] = 'nosniff'
    response.headers['Content-Security-Policy'] = "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' blob:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
    return response


@app.get('/')
def index():
    return app.send_static_file('index.html')


@app.get('/api/health')
def health():
    with db() as c:
        c.execute('SELECT 1').fetchone()
    return jsonify(status='ok')


@app.route('/api/students', methods=['GET', 'POST'])
def students():
    with db() as c:
        if request.method == 'GET':
            query = request.args.get('q', '')
            rows = c.execute('SELECT * FROM students WHERE first_name LIKE ? OR last_name LIKE ? OR national_id LIKE ? ORDER BY created_at DESC LIMIT 500', (f'%{query}%',) * 3).fetchall()
            return jsonify([dict(r) for r in rows])
        b = fields(['first_name', 'last_name', 'age', 'national_id'])
        sid = str(uuid.uuid4())
        c.execute('INSERT INTO students VALUES (?,?,?,?,?,?,?,?,?)', (sid, text(b, 'first_name'), text(b, 'last_name'), integer(b, 'age', 1, 120), text(b, 'national_id', 40), text(b, 'email', 200), text(b, 'phone', 40), None, now()))
        row = c.execute('SELECT * FROM students WHERE id=?', (sid,)).fetchone()
    return jsonify(dict(row)), 201


@app.get('/api/students/<sid>')
def student(sid):
    with db() as c:
        row = c.execute('SELECT * FROM students WHERE id=?', (sid,)).fetchone()
        if not row:
            return jsonify(error='Student not found.'), 404
        result = dict(row)
        result['registrations'] = [dict(r) for r in c.execute('SELECT r.*, COALESCE((SELECT SUM(amount_cents) FROM payments WHERE registration_id=r.id),0) AS paid_cents FROM registrations r WHERE student_id=?', (sid,))]
        result['payments'] = [dict(r) for r in c.execute('SELECT p.* FROM payments p JOIN registrations r ON r.id=p.registration_id WHERE r.student_id=? ORDER BY p.created_at DESC', (sid,))]
    return jsonify(result)


@app.post('/api/students/<sid>/photo')
def photo(sid):
    with db() as c:
        row = c.execute('SELECT * FROM students WHERE id=?', (sid,)).fetchone()
        if not row:
            return jsonify(error='Student not found.'), 404
        upload = request.files.get('photo')
        if not upload:
            raise ValueError('Choose a PNG or JPEG photo.')
        data = upload.read()
        ext = 'png' if data.startswith(b'\x89PNG\r\n\x1a\n') else 'jpg' if data.startswith(b'\xff\xd8\xff') else None
        if not ext:
            raise ValueError('Only PNG and JPEG images are supported.')
        name = f'{uuid.uuid4()}.{ext}'
        (DATA / 'photos' / name).write_bytes(data)
        url = f'/photos/{name}'
        c.execute('UPDATE students SET photo_url=? WHERE id=?', (url, sid))
    return jsonify(photo_url=url), 201


@app.get('/photos/<name>')
def photos(name):
    return send_from_directory(DATA / 'photos', name)


@app.route('/api/registrations', methods=['GET', 'POST'])
def registrations():
    with db() as c:
        if request.method == 'GET':
            return jsonify([dict(r) for r in c.execute('SELECT * FROM registrations ORDER BY created_at DESC LIMIT 500')])
        b = fields(['student_id', 'course', 'semester', 'fee_cents'])
        rid = str(uuid.uuid4())
        c.execute('INSERT INTO registrations VALUES (?,?,?,?,?,?,?)', (rid, text(b, 'student_id'), text(b, 'course'), text(b, 'semester'), 'active', integer(b, 'fee_cents', 0, 100000000), now()))
        result = dict(c.execute('SELECT * FROM registrations WHERE id=?', (rid,)).fetchone())
    return jsonify(result), 201


@app.route('/api/payments', methods=['GET', 'POST'])
def payments():
    with db() as c:
        if request.method == 'GET':
            return jsonify([dict(r) for r in c.execute('SELECT * FROM payments ORDER BY created_at DESC LIMIT 500')])
        b = fields(['registration_id', 'amount_cents', 'method', 'reference'])
        method = text(b, 'method')
        if method not in ['cash', 'bank_transfer', 'card']:
            raise ValueError('Invalid payment method.')
        amount = integer(b, 'amount_cents', 1, 100000000)
        c.execute('BEGIN IMMEDIATE')
        r = c.execute('SELECT fee_cents FROM registrations WHERE id=?', (text(b, 'registration_id'),)).fetchone()
        if not r:
            return jsonify(error='Registration not found.'), 404
        paid = c.execute('SELECT COALESCE(SUM(amount_cents),0) FROM payments WHERE registration_id=?', (b['registration_id'],)).fetchone()[0]
        if amount + paid > r['fee_cents']:
            raise ValueError('Payment exceeds the outstanding balance.')
        pid = str(uuid.uuid4())
        c.execute('INSERT INTO payments VALUES (?,?,?,?,?,?)', (pid, text(b, 'registration_id'), amount, method, text(b, 'reference'), now()))
        result = dict(c.execute('SELECT * FROM payments WHERE id=?', (pid,)).fetchone())
    return jsonify(result), 201


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Student console and web server')
    parser.add_argument('command', choices=['serve', 'list', 'summary'], nargs='?', default='serve')
    args = parser.parse_args()
    if args.command == 'serve':
        app.run(host='0.0.0.0', port=8080)
    else:
        with db() as c:
            if args.command == 'list':
                print(json.dumps([dict(r) for r in c.execute('SELECT * FROM students')], indent=2))
            else:
                print(json.dumps({t: c.execute(f'SELECT COUNT(*) FROM {t}').fetchone()[0] for t in ['students', 'registrations', 'payments']}, indent=2))
