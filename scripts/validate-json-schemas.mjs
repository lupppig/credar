import { readFile } from "node:fs/promises";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const schemaPaths = [
  "contracts/audit/metadata-v1.schema.json",
  "contracts/events/internal/envelope-v1.schema.json",
  "contracts/events/internal/identity-user-linked-v1.schema.json",
  "contracts/events/internal/identity-platform-roles-changed-v1.schema.json",
  "contracts/events/internal/membership-roles-changed-v1.schema.json",
  "contracts/events/internal/membership-status-changed-v1.schema.json",
  "contracts/events/internal/role-changed-v1.schema.json",
  "contracts/events/internal/tenant-created-v1.schema.json",
  "contracts/events/public/envelope-v1.schema.json",
];

const validator = new Ajv2020({ allErrors: true, strict: true });
addFormats(validator);

for (const schemaPath of schemaPaths) {
  const source = await readFile(schemaPath, "utf8");
  const schema = JSON.parse(source);
  validator.compile(schema);
  process.stdout.write(`validated ${schemaPath}\n`);
}
