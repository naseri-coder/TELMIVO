"""Non-production offline capacity tests; no network, database or deletion."""
import importlib.util
from pathlib import Path
import unittest

p=Path(__file__).resolve().parents[2]/"scripts"/"backup"/"estimate_capacity.py"
s=importlib.util.spec_from_file_location("estimate_capacity",p)
m=importlib.util.module_from_spec(s)
s.loader.exec_module(m)

class CapacityTests(unittest.TestCase):
    def test_intervals(self):
        a=m.estimate(10,0,5);b=m.estimate(10,0,10);c=m.estimate(10,0,15)
        self.assertGreater(a["retained_mib_nominal"],b["retained_mib_nominal"])
        self.assertGreater(b["retained_mib_nominal"],c["retained_mib_nominal"])
        self.assertEqual(b["main_daily_generation_mib"],1440)
    def test_daily_license(self):
        x=m.estimate(0,10,10)
        self.assertEqual(x["license_daily_generation_mib"],10)
        self.assertEqual(x["license_retained_points_upper_nominal"],17)
    def test_invalid(self):
        for z in (-1,float("nan"),float("inf")):
            with self.assertRaises(ValueError): m.estimate(z,0,10)
        with self.assertRaises(ValueError): m.estimate(1,1,2)
    def test_unqualified_and_excluded_costs(self):
        x=m.estimate(10,1,10)
        self.assertTrue(any("No deletion" in v for v in x["warnings"]))
        self.assertGreater(x["retained_gib_nominal"],0)

if __name__=="__main__":unittest.main()
