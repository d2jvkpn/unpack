import subprocess
import sys
import tempfile
import unittest
import zipfile
import os
from pathlib import Path


SCRIPT = Path(__file__).with_name("unzip_cn.py")


class UnzipCnCliTests(unittest.TestCase):
    def run_script(self, cwd, *arguments):
        return subprocess.run(
            [sys.executable, str(SCRIPT), *map(str, arguments)],
            cwd=cwd,
            text=True,
            capture_output=True,
            check=False,
        )

    def make_zip(self, path, files):
        with zipfile.ZipFile(path, "w") as archive:
            for name, content in files.items():
                archive.writestr(name, content)

    def test_output_dir_strips_a_single_top_level_directory(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            working_directory = Path(temporary_directory)
            archive_path = working_directory / "archive.zip"
            self.make_zip(
                archive_path,
                {
                    "wrapper/first.txt": "first",
                    "wrapper/nested/second.txt": "second",
                },
            )

            result = self.run_script(
                working_directory, "--output-dir", "result", archive_path
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                (working_directory / "result/first.txt").read_text(), "first"
            )
            self.assertEqual(
                (working_directory / "result/nested/second.txt").read_text(),
                "second",
            )
            self.assertFalse((working_directory / "result/wrapper").exists())

    def test_output_dir_keeps_a_single_top_level_file_name(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            working_directory = Path(temporary_directory)
            archive_path = working_directory / "archive.zip"
            self.make_zip(archive_path, {"only.txt": "content"})

            result = self.run_script(
                working_directory, "--output-dir", "result", archive_path
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                (working_directory / "result/only.txt").read_text(), "content"
            )

    def test_output_dir_keeps_multiple_top_level_items(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            working_directory = Path(temporary_directory)
            archive_path = working_directory / "archive.zip"
            self.make_zip(
                archive_path,
                {
                    "first/one.txt": "one",
                    "second/two.txt": "two",
                },
            )

            result = self.run_script(
                working_directory, "--output-dir", "result", archive_path
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                (working_directory / "result/first/one.txt").read_text(), "one"
            )
            self.assertEqual(
                (working_directory / "result/second/two.txt").read_text(), "two"
            )

    def test_omitting_output_dir_preserves_direct_extraction_behavior(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            working_directory = Path(temporary_directory)
            archive_path = working_directory / "archive.zip"
            self.make_zip(archive_path, {"wrapper/file.txt": "content"})

            result = self.run_script(working_directory, archive_path)

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                (working_directory / "wrapper/file.txt").read_text(), "content"
            )

    def test_archive_members_cannot_escape_output_dir(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            working_directory = Path(temporary_directory)
            archive_path = working_directory / "archive.zip"
            self.make_zip(
                archive_path,
                {"safe.txt": "safe", "../escaped.txt": "escaped"},
            )

            result = self.run_script(
                working_directory, "--output-dir", "result", archive_path
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((working_directory / "escaped.txt").exists())
            self.assertEqual(
                (working_directory / "result/safe.txt").read_text(), "safe"
            )

    def test_archive_members_cannot_escape_through_existing_symlink(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            working_directory = Path(temporary_directory)
            archive_path = working_directory / "archive.zip"
            output_directory = working_directory / "result"
            outside_directory = working_directory / "outside"
            output_directory.mkdir()
            outside_directory.mkdir()
            os.symlink(outside_directory, output_directory / "link")
            self.make_zip(
                archive_path,
                {"safe.txt": "safe", "link/escaped.txt": "escaped"},
            )

            result = self.run_script(
                working_directory, "--output-dir", output_directory, archive_path
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((outside_directory / "escaped.txt").exists())
            self.assertEqual((output_directory / "safe.txt").read_text(), "safe")

    def test_secret_argument_is_accepted_by_zipfile(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            working_directory = Path(temporary_directory)
            archive_path = working_directory / "archive.zip"
            self.make_zip(archive_path, {"file.txt": "content"})

            result = self.run_script(
                working_directory, "--secret", "password", archive_path
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                (working_directory / "file.txt").read_text(), "content"
            )


if __name__ == "__main__":
    unittest.main()
