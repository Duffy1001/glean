#ifndef JSONIFY_WRAPPER_H
#define JSONIFY_WRAPPER_H

#include "llama.h"
#include <stdbool.h>
#include <stddef.h>

typedef struct {
    struct llama_model * model;
    struct llama_context * ctx;
    struct llama_sampler * chain;    // main sampler chain (greedy or temp+dist)
    struct llama_sampler * grammar;  // grammar sampler (NULL if no grammar)
    const struct llama_vocab * vocab;
    llama_token_data * token_data;
    int32_t n_vocab;
    int32_t n_ctx;
} glean_model_t;

// Set log verbosity: 0=none, 1=debug, 2=info, 3=warn, 4=error.
// Messages below this level are suppressed.
void glean_set_log_level(int32_t level);

// Initialize compiled backends and enumerate available devices.
void glean_backend_init(void);

// Free global backend resources after all models have been freed.
void glean_backend_free(void);

// Backend/device reporting.
int32_t glean_backend_count(void);
const char * glean_backend_name(int32_t index);
int32_t glean_backend_device_count(void);
const char * glean_backend_device_name(int32_t index);
const char * glean_backend_device_description(int32_t index);
const char * glean_backend_device_backend(int32_t index);
int32_t glean_backend_device_type(int32_t index);
void glean_backend_device_memory(int32_t index, uint64_t * free_bytes, uint64_t * total_bytes);

// Load model from file path.
glean_model_t * glean_load(const char * model_path, int32_t n_ctx, int32_t n_threads, int32_t n_gpu_layers, bool allow_cpu_fallback);

// Free model and context.
void glean_free(glean_model_t * m);

// Tokenize text. Returns number of tokens written, or -1 on error.
int32_t glean_tokenize(glean_model_t * m, const char * text, int32_t * tokens_buf, int32_t capacity, bool add_bos, bool parse_special);

// Decode tokens. Returns 0 on success.
int32_t glean_decode(glean_model_t * m, const int32_t * tokens, int32_t n_tokens);

// Synchronize (must be called before sampling after decode).
void glean_synchronize(glean_model_t * m);

// Clear the context's KV cache for reuse between independent prompts.
bool glean_clear_context(glean_model_t * m);

// Sample next token using the configured sampler chain.
// Returns the sampled token id. Handles grammar masking internally.
int32_t glean_sample_next(glean_model_t * m);

// Accept a token into the sampler chain (updates grammar state etc).
void glean_accept_token(glean_model_t * m, int32_t token);

// Get vocab size.
int32_t glean_n_vocab(glean_model_t * m);

// Get EOS token id.
int32_t glean_token_eos(glean_model_t * m);

// Detokenize token to text. Returns bytes written.
int32_t glean_token_to_piece(glean_model_t * m, int32_t token, char * buf, int32_t buf_len);

// Set grammar on the model's sampler chain. Returns 0 on success, -1 on failure.
int32_t glean_set_grammar(glean_model_t * m, const char * grammar_str, const char * grammar_root);

// Remove grammar from the model's sampler chain.
void glean_clear_grammar(glean_model_t * m);

// Free a standalone sampler.
void glean_sampler_free(struct llama_sampler * smpl);

// Apply chat template using the model's built-in template.
// Returns a newly allocated string (caller must free). Returns NULL on error.
char *glean_chat_apply_template(glean_model_t * m,
                                  const char * system_msg,
                                  const char * user_msg,
                                  bool add_ass);

#endif
