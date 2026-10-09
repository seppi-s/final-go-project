import base64
import io
import os
import tempfile
import unittest
sandbox = tempfile.TemporaryDirectory()
os.environ['DATA_DIR'] = sandbox.name
from app import app, db

class API(unittest.TestCase):
    def setUp(self):
        self.client=app.test_client()
        with db() as c:
            for t in ['payments','registrations','students']:
                c.execute(f'DELETE FROM {t}')
    def student(self):
        r=self.client.post('/api/students',json={'first_name':'Ada','last_name':'Lovelace','age':21,'national_id':'N123'})
        self.assertEqual(r.status_code,201)
        return r.json['id']
    def test_students(self):
        sid=self.student()
        self.assertEqual(len(self.client.get('/api/students?q=Ada').json),1)
        self.assertEqual(self.client.get('/api/students/'+sid).json['last_name'],'Lovelace')
        b={'first_name':'A','last_name':'B','age':True,'national_id':'N2'}
        self.assertEqual(self.client.post('/api/students',json=b).status_code,400)
        b.update(age=22,national_id='N123')
        self.assertEqual(self.client.post('/api/students',json=b).status_code,409)
        self.assertEqual(self.client.get('/api/students/missing').status_code,404)
        with db() as c:
            self.assertEqual(c.execute('SELECT COUNT(*) FROM students').fetchone()[0],1)
    def test_payments(self):
        sid=self.student()
        b={'student_id':sid,'course':'CS','semester':'Fall 2026','fee_cents':10000}
        r=self.client.post('/api/registrations',json=b)
        self.assertEqual(r.status_code,201)
        self.assertEqual(self.client.post('/api/registrations',json=b).status_code,409)
        p={'registration_id':r.json['id'],'amount_cents':6000,'method':'cash','reference':'R1'}
        self.assertEqual(self.client.post('/api/payments',json=p).status_code,201)
        p.update(amount_cents=100,reference='R1')
        self.assertEqual(self.client.post('/api/payments',json=p).status_code,409)
        p.update(amount_cents=5000,reference='R2')
        self.assertEqual(self.client.post('/api/payments',json=p).status_code,400)
        p.update(amount_cents=4000)
        self.assertEqual(self.client.post('/api/payments',json=p).status_code,201)
        self.assertEqual(self.client.get('/api/students/'+sid).json['registrations'][0]['paid_cents'],10000)
        self.assertEqual(len(self.client.get('/api/payments').json),2)
        self.assertEqual(len(self.client.get('/api/registrations').json),1)
        b.update(student_id='missing',course='Other')
        self.assertEqual(self.client.post('/api/registrations',json=b).status_code,409)
    def test_photo_and_health(self):
        sid=self.student()
        png=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=')
        url='/api/students/'+sid+'/photo'
        r=self.client.post(url,data={'photo':(io.BytesIO(png),'../../p.png')})
        self.assertEqual(r.status_code,201)
        with self.client.get(r.json['photo_url']) as response:
            self.assertEqual(response.data,png)
        self.assertEqual(self.client.post(url,data={'photo':(io.BytesIO(b'<script>'),'bad.png')}).status_code,400)
        self.assertEqual(self.client.post(url,data={'photo':(io.BytesIO(b'x'*5300000),'large.png')}).status_code,413)
        self.assertEqual(self.client.get('/api/health').json,{'status':'ok'})
        with self.client.get('/') as response:
            self.assertIn("default-src 'self'",response.headers['Content-Security-Policy'])
