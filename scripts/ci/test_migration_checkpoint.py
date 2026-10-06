from pathlib import Path
import unittest
from migration_checkpoint import validate_chain

class MigrationCheckpointTests(unittest.TestCase):
    def setUp(self):
        root = Path(__file__).resolve().parents[2]
        self.names = sorted(p.name for p in (root / 'db/migrations').glob('*.sql'))

    def test_historical_and_current_boundaries_are_distinct(self):
        validate_chain(self.names, 'R5C')
        validate_chain(self.names[:26], 'R5B_REMEDIATED')
        validate_chain(self.names[:25], 'R5B')
        validate_chain(self.names[:23], 'R4G')
        for names, checkpoint in [(self.names, 'R4G'), (self.names[:23], 'R5B'), (self.names, 'unknown')]:
            with self.assertRaises(ValueError):
                validate_chain(names, checkpoint)

    def test_missing_duplicate_order_and_future_fail(self):
        duplicate = self.names[:]
        duplicate[10] = duplicate[9]
        order = self.names[:]
        order[0], order[1] = order[1], order[0]
        final = self.names[:]
        final[-1] = '0027_wrong.sql'
        for names in [self.names[:10] + self.names[11:], duplicate, order, final,
                      self.names + ['0028_unexpected.sql']]:
            with self.subTest(names=names), self.assertRaises(ValueError):
                validate_chain(names, 'R5C')

if __name__ == '__main__':
    unittest.main()
