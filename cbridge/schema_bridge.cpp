#include "schema_bridge.h"
#include "json-schema-to-grammar.h"
#include <nlohmann/json.hpp>
#include <cstdlib>
#include <cstring>
#include <string>

using json = nlohmann::ordered_json;

extern "C"
char *jsonify_schema_to_grammar(const char *json_schema, char **err_msg) {
    if (err_msg) *err_msg = NULL;

    if (!json_schema) {
        if (err_msg) *err_msg = strdup("null schema input");
        return NULL;
    }

    try {
        json schema = json::parse(json_schema);
        std::string gbnf = json_schema_to_grammar(schema, /*force_gbnf=*/true);
        char *result = strdup(gbnf.c_str());
        return result;
    } catch (const std::exception &e) {
        if (err_msg) *err_msg = strdup(e.what());
        return NULL;
    }
}
