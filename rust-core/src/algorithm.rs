//! Algorithm framework for processing data

use crate::memory::MemoryManager;
use serde::{Serialize, Deserialize};

/// Trait for algorithm implementation
pub trait Algorithm {
    /// Process input data and return output
    fn process(&self, input: &[u8], memory: &mut MemoryManager) -> Result<Vec<u8>, String>;
    
    /// Get the algorithm's unique identifier
    fn id(&self) -> &str;
    
    /// Get the algorithm's metadata
    fn metadata(&self) -> AlgorithmMetadata;
}

/// Metadata for algorithm description and configuration
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct AlgorithmMetadata {
    pub name: String,
    pub version: String,
    pub description: String,
    pub parameters: Vec<ParameterDefinition>,
}

/// Parameter definition for algorithm configuration
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct ParameterDefinition {
    pub name: String,
    pub parameter_type: ParameterType,
    pub description: String,
    pub default_value: Option<String>,
}

/// Types of parameters supported in algorithms
#[derive(Clone, Debug, Serialize, Deserialize)]
pub enum ParameterType {
    Integer,
    Float,
    Boolean,
    String,
    Array,
    Object,
}

/// Factory function to get algorithm by ID
pub fn get_algorithm_by_id(algorithm_id: &str) -> Option<Box<dyn Algorithm>> {
    // This would be populated with registered algorithms
    // For now, return None as we haven't implemented any concrete algorithms
    None
}

/// Create an algorithm from JSON definition
pub fn create_algorithm_from_json(json_definition: &str) -> Result<Box<dyn Algorithm>, String> {
    // Parse JSON and create a dynamic algorithm
    // This is a placeholder for the actual implementation
    Err("Not implemented yet".to_string())
}
