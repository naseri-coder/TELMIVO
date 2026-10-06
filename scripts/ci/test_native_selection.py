import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('native_selection', Path(__file__).with_name('check-native-selection.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class NativeSelectionTests(unittest.TestCase):
    def test_current_contract(self):
        root = module.ROOT
        selected = module.selection((root / '.github/workflows/migration-check.yml').read_text(),
                                    (root / 'web/miniapp/playwright.native.config.ts').read_text())
        self.assertIn('automation2.native.spec.ts', selected)
        self.assertIn('ai2.native.spec.ts', selected)
        self.assertEqual(14, len(selected))

    def test_missing_extra_duplicate_and_broken_definition_fail(self):
        workflow = 'for spec in ai2.native.spec.ts automation2.native.spec.ts; do'
        for config in ["testMatch:['ai2.native.spec.ts']", "testMatch:[]",
                       "testMatch:['ai2.native.spec.ts','automation2.native.spec.ts','unused.spec.ts']",
                       "testMatch:['ai2.native.spec.ts','ai2.native.spec.ts','automation2.native.spec.ts']",
                       'testMatch:undefined']:
            with self.subTest(config=config), self.assertRaises(ValueError):
                module.selection(workflow, config)
        with self.assertRaises(ValueError):
            module.selection('for spec in ; do', "testMatch:[]")

if __name__ == '__main__':
    unittest.main()
