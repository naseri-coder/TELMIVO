import importlib.util
import io
import unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('extract','scripts/media/extract.py')
x=importlib.util.module_from_spec(spec);spec.loader.exec_module(x)
class GuardTests(unittest.TestCase):
 def test_private_dns(self):
  for address in ('127.0.0.1','10.0.0.1','169.254.169.254','100.100.100.200','::1','fd00::1'):
   with self.subTest(address=address),patch.object(x.socket,'getaddrinfo',return_value=[(2,1,6,'',(address,443))]):
    with self.assertRaises(ValueError):x.allowed('youtube','https://www.youtube.com/watch?v=dQw4w9WgXc')
 def test_unsupported_before_dns(self):
  with patch.object(x.socket,'getaddrinfo') as lookup:
   with self.assertRaises(ValueError):x.allowed('youtube','https://evil.test/a')
   lookup.assert_not_called()
 def test_rebinding_each_request(self):
  with patch.object(x.socket,'getaddrinfo',side_effect=[[(2,1,6,'',('8.8.8.8',443))],[(2,1,6,'',('127.0.0.1',443))]]):
   self.assertEqual(x.allowed('youtube','https://youtube.com/a')[1],'8.8.8.8')
   with self.assertRaises(ValueError):x.allowed('youtube','https://youtube.com/a')
 def test_pin_and_redirect(self):
  class Reply:
   status=302
   def getheader(self,*a):return 'https://169.254.169.254/secret'
  class Conn:
   def __init__(self,host,address):self.host=host;self.address=address
   def request(self,*a,**k):pass
   def getresponse(self):return Reply()
   def close(self):pass
  with patch.object(x.socket,'getaddrinfo',return_value=[(2,1,6,'',('8.8.8.8',443))]),patch.object(x,'PinnedTLS',Conn):
   with x.GuardedDL('youtube') as ydl:
    with self.assertRaises(x.TransportError):ydl.urlopen('https://youtube.com/a')
if __name__=='__main__':unittest.main()
