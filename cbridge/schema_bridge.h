#ifndef JSONIFY_SCHEMA_BRIDGE_H
#define JSONIFY_SCHEMA_BRIDGE_H

#ifdef __cplusplus
extern "C" {
#endif

// Convert a JSON Schema string to GBNF grammar.
// On success: returns a newly allocated C string (caller must free).
//             *err_msg is set to NULL.
// On failure: returns NULL, *err_msg is set to a newly allocated error string
//             (caller must free). If err_msg is NULL, no error message is returned.
char *glean_schema_to_grammar(const char *json_schema, char **err_msg);

#ifdef __cplusplus
}
#endif

#endif
